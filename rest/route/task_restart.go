package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
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
			Message:    "task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	if projCtx.ProjectRef == nil {
		return gimlet.ErrorResponse{
			Message:    "project not found",
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
	err := resetTask(trh.taskId, trh.username)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "restarting task"))
	}

	refreshedTask, err := task.FindOneId(trh.taskId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding updated task"))
	}
	if refreshedTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", trh.taskId),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromArgs(refreshedTask, &model.APITaskArgs{IncludeProjectIdentifier: true, IncludeAMI: true})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task to API model"))
	}
	return gimlet.NewJSONResponse(taskModel)
}

// resetTask sets the task to be in an unexecuted state and prepares it to be run again.
// If given an execution task, marks the display task for reset.
func resetTask(taskId, username string) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", t)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", t.Id)
	}
	return errors.Wrap(serviceModel.ResetTaskOrDisplayTask(t, username, evergreen.RESTV2Package, nil), "resetting task")
}
