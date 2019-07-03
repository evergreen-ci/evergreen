package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskRestartHandler implements the route POST /task/{task_id}/restart. It
// fetches the needed task and project and calls the service function to
// set the proper fields when reseting the task.
type taskRestartHandler struct {
	taskId   string
	username string
	sc       data.Connector
}

func makeTaskRestartHandler(sc data.Connector) gimlet.RouteHandler {
	return &taskRestartHandler{
		sc: sc,
	}
}

func (trh *taskRestartHandler) Factory() gimlet.RouteHandler {
	return &taskRestartHandler{
		sc: trh.sc,
	}
}

// ParseAndValidate fetches the taskId and Project from the request context and
// sets them on the taskRestartHandler to be used by Execute.
func (trh *taskRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	if projCtx.ProjectRef == nil {
		return gimlet.ErrorResponse{
			Message:    "Project not found",
			StatusCode: http.StatusNotFound,
		}
	}
	trh.taskId = projCtx.Task.Id
	u := MustHaveUser(ctx)
	trh.username = u.DisplayName()
	return nil
}

// Execute calls the data ResetTask function and returns the refreshed
// task from the service.
func (trh *taskRestartHandler) Run(ctx context.Context) gimlet.Responder {
	err := trh.sc.ResetTask(trh.taskId, trh.username)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	refreshedTask, err := trh.sc.FindTaskById(trh.taskId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(taskModel)
}
