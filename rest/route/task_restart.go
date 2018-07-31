package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func getTaskRestartRouteManager(route string, version int) *RouteManager {
	trh := &taskRestartHandler{}
	taskRestart := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchProjectContext},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    trh.Handler(),
		MethodType:        http.MethodPost,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{taskRestart},
		Version: version,
	}
	return &taskRoute
}

// taskRestartHandler implements the route POST /task/{task_id}/restart. It
// fetches the needed task and project and calls the service function to
// set the proper fields when reseting the task.
type taskRestartHandler struct {
	taskId string

	username string
}

func (trh *taskRestartHandler) Handler() RequestHandler {
	return &taskRestartHandler{}
}

// ParseAndValidate fetches the taskId and Project from the request context and
// sets them on the taskRestartHandler to be used by Execute.
func (trh *taskRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
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
func (trh *taskRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.ResetTask(trh.taskId, trh.username)
	if err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		}
	}

	refreshedTask, err := sc.FindTaskById(trh.taskId)
	if err != nil {
		return ResponseData{}, err
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}
