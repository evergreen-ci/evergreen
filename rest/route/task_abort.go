package route

import (
	"context"
	"fmt"
	"net/http"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
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
	err := serviceModel.AbortTask(t.taskId, MustHaveUser(ctx).Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "aborting task"))
	}

	foundTask, err := task.FindOneId(t.taskId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding updated task"))
	}
	if foundTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", t.taskId),
		})
	}
	taskModel := &model.APITask{}

	if err = taskModel.BuildFromArgs(foundTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
	}); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "converting task to API model"))
	}
	return gimlet.NewJSONResponse(taskModel)
}
