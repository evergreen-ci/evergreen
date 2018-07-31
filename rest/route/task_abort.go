package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type taskAbortHandler struct {
	taskId string
}

func getTaskAbortManager(route string, version int) *RouteManager {
	t := &taskAbortHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: t.Handler(),
			},
		},
	}
}

func (t *taskAbortHandler) Handler() RequestHandler {
	return &taskAbortHandler{}
}

func (t *taskAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	t.taskId = gimlet.GetVars(r)["task_id"]
	return nil
}

func (t *taskAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.AbortTask(t.taskId, MustHaveUser(ctx).Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Abort error")
	}

	foundTask, err := sc.FindTaskById(t.taskId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(foundTask)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}
