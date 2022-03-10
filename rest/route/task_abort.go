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

func makeTaskAbortHandler() gimlet.RouteHandler {
	return &taskAbortHandler{}
}

func (t *taskAbortHandler) Factory() gimlet.RouteHandler {
	return &taskAbortHandler{}
}

func (t *taskAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	t.taskId = gimlet.GetVars(r)["task_id"]
	return nil
}

func (t *taskAbortHandler) Run(ctx context.Context) gimlet.Responder {
	err := data.AbortTask(t.taskId, MustHaveUser(ctx).Id)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Abort error"))
	}

	foundTask, err := data.FindTaskById(t.taskId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}
	taskModel := &model.APITask{}

	if err = taskModel.BuildFromService(foundTask); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(taskModel)
}
