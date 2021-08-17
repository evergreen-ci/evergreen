package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/utility"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type tasksByBuildHandler struct {
	buildId            string
	status             string
	fetchAllExecutions bool
	fetchParentIds     bool
	sc                 data.Connector
	limit              int
	key                string
}

func makeFetchTasksByBuild(sc data.Connector) gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit: defaultLimit,
		sc:    sc,
	}
}

func (tbh *tasksByBuildHandler) Factory() gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit: tbh.limit,
		sc:    tbh.sc,
	}
}

func (tbh *tasksByBuildHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	tbh.buildId = gimlet.GetVars(r)["build_id"]
	if tbh.buildId == "" {
		return gimlet.ErrorResponse{
			Message:    "buildId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	tbh.status = vals.Get("status")
	tbh.key = vals.Get("start_at")

	var err error
	tbh.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	tbh.fetchAllExecutions = vals.Get("fetch_all_executions") == "true"
	tbh.fetchParentIds = vals.Get("fetch_parent_ids") == "true"

	return nil
}

func (tbh *tasksByBuildHandler) Run(ctx context.Context) gimlet.Responder {
	// Fetch all of the tasks to be returned in this page plus the tasks used for
	// calculating information about the next page. Here the limit is multiplied
	// by two to fetch the next page.
	tasks, err := tbh.sc.FindTasksByBuildId(tbh.buildId, tbh.key, tbh.status, tbh.limit+1, 1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(tasks)
	if len(tasks) > tbh.limit {
		lastIndex = tbh.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tbh.sc.GetURL(),
				Key:             tasks[tbh.limit].Id,
				Limit:           tbh.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	tasks = tasks[:lastIndex]
	for i := range tasks {
		taskModel := &model.APITask{}

		if err = taskModel.BuildFromService(&tasks[i]); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		if err = taskModel.GetArtifacts(); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		if err = taskModel.BuildFromService(tbh.sc.GetURL()); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		if tbh.fetchAllExecutions {
			var oldTasks []task.Task

			oldTasks, err = tbh.sc.FindOldTasksByIDWithDisplayTasks(tasks[i].Id)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}

			if err = taskModel.BuildPreviousExecutions(oldTasks, tbh.sc.GetURL()); err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
		}

		if tbh.fetchParentIds {
			if tasks[i].IsPartOfDisplay() {
				taskModel.ParentTaskId = utility.FromStringPtr(tasks[i].DisplayTaskId)
			}
		}

		if err = resp.AddData(taskModel); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
