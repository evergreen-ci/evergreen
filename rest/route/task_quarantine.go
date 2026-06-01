package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func parseTaskForQuarantine(ctx context.Context) (*task.Task, error) {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "task not found",
		}
	}
	if projCtx.Task.DisplayOnly {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot modify quarantine state on display task '%s', select an execution task instead", projCtx.Task.Id),
		}
	}
	return projCtx.Task, nil
}

func quarantineTask(ctx context.Context, t *task.Task, isManuallyQuarantined bool) gimlet.Responder {
	u := MustHaveUser(ctx)
	if err := data.SetTaskQuarantined(ctx, t.Project, t.BuildVariant, t.DisplayName, isManuallyQuarantined); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting quarantine state to '%t' for task '%s'", isManuallyQuarantined, t.Id))
	}
	grip.Info(ctx, message.Fields{
		"message":                 "task quarantine state changed",
		"user":                    u.Username(),
		"task_id":                 t.Id,
		"project":                 t.Project,
		"build_variant":           t.BuildVariant,
		"task_name":               t.DisplayName,
		"is_manually_quarantined": isManuallyQuarantined,
	})
	apiTask := &model.APITask{}
	if err := apiTask.BuildFromService(ctx, t, &model.APITaskArgs{IncludeProjectIdentifier: true}); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", t.Id))
	}
	return gimlet.NewJSONResponse(apiTask)
}

type taskQuarantineHandler struct {
	task *task.Task
}

func makeTaskQuarantineHandler() gimlet.RouteHandler {
	return &taskQuarantineHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Quarantine a task
//	@Description	Marks all known tests of the given task as manually quarantined in the test selection service.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/quarantine [post]
//	@Security		Api-User || Api-Key
//	@Param			task_id	path		string	true	"the task ID"
//	@Success		200		{object}	model.APITask
func (h *taskQuarantineHandler) Factory() gimlet.RouteHandler {
	return &taskQuarantineHandler{}
}

func (h *taskQuarantineHandler) Parse(ctx context.Context, r *http.Request) error {
	t, err := parseTaskForQuarantine(ctx)
	if err != nil {
		return err
	}
	h.task = t
	return nil
}

func (h *taskQuarantineHandler) Run(ctx context.Context) gimlet.Responder {
	return quarantineTask(ctx, h.task, true)
}

type taskUnquarantineHandler struct {
	task *task.Task
}

func makeTaskUnquarantineHandler() gimlet.RouteHandler {
	return &taskUnquarantineHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Unquarantine a task
//	@Description	Marks all known tests of the given task as no longer manually quarantined in the test selection service.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/unquarantine [post]
//	@Security		Api-User || Api-Key
//	@Param			task_id	path		string	true	"the task ID"
//	@Success		200		{object}	model.APITask
func (h *taskUnquarantineHandler) Factory() gimlet.RouteHandler {
	return &taskUnquarantineHandler{}
}

func (h *taskUnquarantineHandler) Parse(ctx context.Context, r *http.Request) error {
	t, err := parseTaskForQuarantine(ctx)
	if err != nil {
		return err
	}
	h.task = t
	return nil
}

func (h *taskUnquarantineHandler) Run(ctx context.Context) gimlet.Responder {
	return quarantineTask(ctx, h.task, false)
}
