package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskRestartHandler implements the route POST /tasks/{task_id}/restart. It
// fetches the task and project and calls the service function to restart the
// task (or display task).
//
// A display task is the schedulable unit, so restarting a display task restarts
// its execution tasks. Two mutually exclusive options scope the restart to a
// subset of a display task's execution tasks:
//
//   - FailedOnly restarts only the execution tasks that failed.
//   - ExecutionTaskIDs restarts only the listed execution tasks, regardless of
//     whether they failed.
//
// A scoped restart (FailedOnly or ExecutionTaskIDs) is deferred until every
// execution task in the display task is finished, because the display task
// cannot be re-archived while an execution task may still be running. When the
// reset happens, the display task's execution is incremented once and only the
// selected execution tasks are rerun; unselected execution tasks keep their
// existing results.
//
// Scoped restart requests for the same display task that are made while a reset
// is pending are merged into a single reset. A full display task restart
// supersedes a pending scoped restart, and a scoped restart is a no-op if a full
// restart is already pending. ExecutionTaskIDs is ignored for a non-display
// task.
type taskRestartHandler struct {
	// If set for a display task, restarts only failed execution tasks. When
	// used with a non-display task, this parameter has no effect. Mutually
	// exclusive with ExecutionTaskIDs.
	FailedOnly bool `json:"failed_only"`
	// If set for a display task, restarts only the execution tasks with these
	// IDs, regardless of whether they failed. Mutually exclusive with
	// FailedOnly. Each ID must be an execution task of the display task; an ID
	// that is not part of the display task results in a 400 error.
	ExecutionTaskIDs []string `json:"execution_task_ids,omitempty"`

	taskId   string
	username string
}

func makeTaskRestartHandler() gimlet.RouteHandler {
	return &taskRestartHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Restart a task
//	@Description	Restarts the given task. For a display task (or an execution task, which resolves to its display task), the restart reruns the display task's execution tasks; "failed_only" limits this to failed execution tasks and "execution_task_ids" to the listed execution tasks (mutually exclusive, ignored for non-display tasks). Display task resets are deferred until every execution task is finished, and scoped resets arriving while a reset is pending are merged; only a currently running non-display task is rejected.
//	@Tags			tasks
//	@Router			/tasks/{task_id}/restart [post]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path		string				true	"task ID"
//	@Param			{object}	body		taskRestartHandler	false	"parameters"
//	@Success		200			{object}	model.APITask
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
	trh.username = u.Username()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrapf(err, "reading body")
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, trh); err != nil {
			return errors.Wrapf(err, "parsing request's body as JSON for following task ID: '%s'.", trh.taskId)
		}
	}

	return nil
}

// Execute calls the data ResetTask function and returns the refreshed
// task from the service.
func (trh *taskRestartHandler) Run(ctx context.Context) gimlet.Responder {
	err := resetTask(ctx, evergreen.GetEnvironment().Settings(), trh.taskId, trh.username, trh.FailedOnly, trh.ExecutionTaskIDs)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	refreshedTask, err := task.FindOneId(ctx, trh.taskId)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding updated task '%s'", trh.taskId))
	}
	if refreshedTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", trh.taskId),
		})
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(ctx, refreshedTask, &model.APITaskArgs{IncludeProjectIdentifier: true, IncludeAMI: true})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting task '%s' to API model", trh.taskId))
	}
	return gimlet.NewJSONResponse(taskModel)
}

// resetTask sets the task to be in an unexecuted state and prepares it to be run again.
// If given an execution task, marks the display task for reset. If execTaskIDs is set, only
// those execution tasks of the display task are restarted.
func resetTask(ctx context.Context, settings *evergreen.Settings, taskId, username string, failedOnly bool, execTaskIDs []string) error {
	t, err := task.FindOneId(ctx, taskId)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding task '%s'", t).Error(),
		}
	}
	if t == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", taskId),
		}
	}
	if failedOnly && len(execTaskIDs) > 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot restart only failed execution tasks and a specific set of execution tasks at the same time",
		}
	}
	if err := serviceModel.ValidateExecutionTasksToRestart(t, execTaskIDs); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	return errors.Wrapf(serviceModel.ResetTaskOrDisplayTask(ctx, settings, t, serviceModel.ResetTaskOptions{
		User:             username,
		Origin:           evergreen.RESTV2Package,
		FailedOnly:       failedOnly,
		ExecutionTaskIDs: execTaskIDs,
	}), "resetting task '%s'", taskId)
}
