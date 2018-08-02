package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskByProjectHandler implements the GET /projects/{project_id}/revisions/{commit_hash}/tasks.
// It fetches the associated tasks and returns them to the user.
type tasksByProjectHandler struct {
	projectId  string
	commitHash string
	status     string
	limit      int
	key        string
	sc         data.Connector
}

func makeTasksByProjectAndCommitHandler(sc data.Connector) gimlet.RouteHandler {
	return &tasksByProjectHandler{
		sc: sc,
	}
}

func (tph *tasksByProjectHandler) Factory() gimlet.RouteHandler {
	return &tasksByProjectHandler{sc: tph.sc}
}

// ParseAndValidate fetches the project context and task status from the request
// and loads them into the arguments to be used by the execution.
func (tph *tasksByProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	tph.projectId = vars["project_id"]
	tph.commitHash = vars["commit_hash"]
	tph.status = r.URL.Query().Get("status")

	if tph.projectId == "" {
		return gimlet.ErrorResponse{
			Message:    "ProjectId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	if tph.commitHash == "" {
		return gimlet.ErrorResponse{
			Message:    "Revision cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	vals := r.URL.Query()

	tph.key = vals.Get("start_at")

	var err error
	tph.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (tph *tasksByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	tasks, err := tph.sc.FindTasksByProjectAndCommit(tph.projectId, tph.commitHash, tph.key, tph.status, tph.limit+1)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(tasks)
	if len(tasks) > tph.limit {
		lastIndex = tph.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tph.sc.GetURL(),
				Key:             tasks[tph.limit].Id,
				Limit:           tph.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	tasks = tasks[:lastIndex]

	for _, t := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromService(&t)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		err = taskModel.BuildFromService(tph.sc.GetURL())
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		err = resp.AddData(taskModel)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
