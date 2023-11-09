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

// Factory creates an instance of the handler.
//
//	@Summary		Abort a task
//	@Description	Abort the task of the given ID. Can only be performed if the task is in progress.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/abort [post]
//	@Security		Api-User || Api-Key
//	@Param			task_id	path		string	true	"task ID"
//	@Success		200		{object}	model.APITask
func (t *taskAbortHandler) Factory() gimlet.RouteHandler {
	return &taskAbortHandler{}
}

func (t *taskAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	t.taskId = gimlet.GetVars(r)["task_id"]
	return nil
}

func (t *taskAbortHandler) Run(ctx context.Context) gimlet.Responder {
	err := serviceModel.AbortTask(ctx, t.taskId, MustHaveUser(ctx).Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "aborting task '%s'", t.taskId))
	}

	foundTask, err := task.FindOneId(t.taskId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding updated task '%s'", t.taskId))
	}
	if foundTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", t.taskId),
		})
	}
	taskModel := &model.APITask{}

	if err = taskModel.BuildFromService(ctx, foundTask, &model.APITaskArgs{
		IncludeProjectIdentifier: true,
		IncludeAMI:               true,
	}); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", t.taskId))
	}
	return gimlet.NewJSONResponse(taskModel)
}
