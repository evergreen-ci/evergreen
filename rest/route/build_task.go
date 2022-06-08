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
	limit              int
	key                string
	url                string
}

func makeFetchTasksByBuild(url string) gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit: defaultLimit,
		url:   url,
	}
}

func (tbh *tasksByBuildHandler) Factory() gimlet.RouteHandler {
	return &tasksByBuildHandler{
		limit: tbh.limit,
		url:   tbh.url,
	}
}

func (tbh *tasksByBuildHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	tbh.buildId = gimlet.GetVars(r)["build_id"]
	if tbh.buildId == "" {
		return errors.New("build ID cannot be empty")
	}

	tbh.status = vals.Get("status")
	tbh.key = vals.Get("start_at")

	var err error
	tbh.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "getting limit")
	}

	tbh.fetchAllExecutions = vals.Get("fetch_all_executions") == "true"
	tbh.fetchParentIds = vals.Get("fetch_parent_ids") == "true"

	return nil
}

func (tbh *tasksByBuildHandler) Run(ctx context.Context) gimlet.Responder {
	// Fetch all of the tasks to be returned in this page plus the tasks used for
	// calculating information about the next page. Here the limit is multiplied
	// by two to fetch the next page.
	tasks, err := data.FindTasksByBuildId(tbh.buildId, tbh.key, tbh.status, tbh.limit+1, 1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding tasks for build '%s'", tbh.buildId))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "setting JSON response format"))
	}

	lastIndex := len(tasks)
	if len(tasks) > tbh.limit {
		lastIndex = tbh.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tbh.url,
				Key:             tasks[tbh.limit].Id,
				Limit:           tbh.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	tasks = tasks[:lastIndex]
	for i := range tasks {
		taskModel := &model.APITask{}

		if err = taskModel.BuildFromArgs(&tasks[i], &model.APITaskArgs{
			IncludeAMI:               true,
			IncludeArtifacts:         true,
			IncludeProjectIdentifier: true,
			LogURL:                   tbh.url,
		}); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", tasks[i].Id))
		}

		if tbh.fetchAllExecutions {
			var oldTasks []task.Task

			oldTasks, err = task.FindOldWithDisplayTasks(task.ByOldTaskID(tasks[i].Id))
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding archived task '%s'", tasks[i].Id))
			}

			if err = taskModel.BuildPreviousExecutions(oldTasks, tbh.url); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding previous task executions to API model"))
			}
		}

		if tbh.fetchParentIds {
			if tasks[i].IsPartOfDisplay() {
				taskModel.ParentTaskId = utility.FromStringPtr(tasks[i].DisplayTaskId)
			}
		}

		if err = resp.AddData(taskModel); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding response data"))
		}
	}

	return resp
}
