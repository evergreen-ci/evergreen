package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
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
}

func makeTaskRestartHandler() gimlet.RouteHandler {
	return &taskRestartHandler{}
}

func (trh *taskRestartHandler) Factory() gimlet.RouteHandler {
	return &taskRestartHandler{}
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
	err := data.ResetTask(trh.taskId, trh.username)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	refreshedTask, err := task.FindOneId(trh.taskId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if refreshedTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task with id %s not found", trh.taskId),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(taskModel)
}
