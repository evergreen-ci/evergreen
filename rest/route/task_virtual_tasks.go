package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const maxVirtualTaskCompletionBatchSize = 100

// POST /task/{task_id}/virtual_tasks/complete
type completeVirtualTasksHandler struct {
	env    evergreen.Environment
	taskID string
	body   apimodels.CompleteVirtualTasksRequest
}

func makeCompleteVirtualTasks(env evergreen.Environment) gimlet.RouteHandler {
	return &completeVirtualTasksHandler{env: env}
}

func (h *completeVirtualTasksHandler) Factory() gimlet.RouteHandler {
	return &completeVirtualTasksHandler{env: h.env}
}

func (h *completeVirtualTasksHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.body); err != nil {
		return errors.Wrapf(err, "reading virtual task completions for task '%s'", h.taskID)
	}
	if len(h.body.Tasks) == 0 {
		return errors.New("must specify at least one task to complete")
	}
	if len(h.body.Tasks) > maxVirtualTaskCompletionBatchSize {
		return errors.Errorf("batch size %d exceeds the maximum of %d", len(h.body.Tasks), maxVirtualTaskCompletionBatchSize)
	}
	return nil
}

func (h *completeVirtualTasksHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting service flags"))
	}
	if flags.VirtualTasksDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "virtual tasks are disabled",
		})
	}

	// For task auth, the authenticated task is the runner. For user auth
	// (service users), the URL task ID identifies the runner on whose behalf
	// the user is pushing results.
	runner := GetTask(ctx)
	if runner == nil {
		runner, err = task.FindOneId(ctx, h.taskID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
		}
		if runner == nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task '%s' not found", h.taskID),
			})
		}
	}

	pRef, err := model.FindMergedProjectRef(ctx, runner.Project, runner.Version, false)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", runner.Project))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", runner.Project),
		})
	}
	if !pRef.IsVirtualTasksEnabled() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("virtual tasks are not enabled for project '%s'", pRef.Identifier),
		})
	}

	// Task-authenticated requests have no user attached.
	if usr := gimlet.GetUser(ctx); usr != nil {
		if !usr.HasPermission(ctx, gimlet.PermissionOpts{
			Resource:      runner.Project,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusForbidden,
				Message:    fmt.Sprintf("user '%s' does not have permission to complete virtual tasks in project '%s'", usr.Username(), pRef.Identifier),
			})
		}
	}

	resp := apimodels.CompleteVirtualTasksResponse{}
	for _, completion := range h.body.Tasks {
		result := h.completeTask(ctx, runner, completion)
		grip.Info(ctx, message.Fields{
			"message":   "virtual task push completion",
			"runner":    runner.Id,
			"task_id":   completion.TaskID,
			"outcome":   result.Outcome,
			"reason":    result.Reason,
			"execution": completion.Execution,
		})
		resp.Results = append(resp.Results, result)
	}

	responder := gimlet.NewJSONResponse(resp)
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting response status"))
	}
	return responder
}

// completeTask push-completes a single virtual task and returns its outcome.
func (h *completeVirtualTasksHandler) completeTask(ctx context.Context, runner *task.Task, completion apimodels.VirtualTaskCompletion) apimodels.VirtualTaskCompletionResult {
	failed := func(reason string) apimodels.VirtualTaskCompletionResult {
		return apimodels.VirtualTaskCompletionResult{
			TaskID:  completion.TaskID,
			Outcome: apimodels.VirtualTaskCompletionOutcomeFailed,
			Reason:  reason,
		}
	}
	// No-ops return success so that runners don't retry idempotent pushes.
	successNoop := func(reason string) apimodels.VirtualTaskCompletionResult {
		return apimodels.VirtualTaskCompletionResult{
			TaskID:  completion.TaskID,
			Outcome: apimodels.VirtualTaskCompletionOutcomeSuccess,
			Reason:  reason,
		}
	}

	if !evergreen.IsValidTaskEndStatus(completion.Status) {
		return failed(fmt.Sprintf("invalid status '%s', must be '%s' or '%s'", completion.Status, evergreen.TaskSucceeded, evergreen.TaskFailed))
	}
	if tr := completion.TestResults; tr != nil && (tr.Stats.FailedCount < 0 || tr.Stats.TotalCount < 0) {
		return failed("test results stats counts cannot be negative")
	}
	files := make([]artifact.File, 0, len(completion.Artifacts))
	for _, a := range completion.Artifacts {
		if a.Name == "" || a.URL == "" {
			return failed("artifact name and URL must be non-empty")
		}
		if !utility.StringSliceContains(artifact.ValidVisibilities, a.Visibility) {
			return failed(fmt.Sprintf("invalid visibility '%s' for artifact '%s'", a.Visibility, a.Name))
		}
		files = append(files, artifact.File{
			Name:       a.Name,
			Link:       a.URL,
			Visibility: a.Visibility,
		})
	}

	vt, err := task.FindOneId(ctx, completion.TaskID)
	if err != nil {
		return failed(errors.Wrap(err, "finding task").Error())
	}
	if vt == nil {
		return failed("task not found")
	}
	if vt.ExecutionPlatform != task.ExecutionPlatformVirtual {
		return failed("task is not a virtual task")
	}
	if vt.Version != runner.Version {
		return failed(fmt.Sprintf("task does not belong to the same version as task '%s'", runner.Id))
	}

	if vt.Execution != completion.Execution {
		return successNoop(fmt.Sprintf("completion is for execution %d but the task is on execution %d", completion.Execution, vt.Execution))
	}
	if vt.IsFinished() {
		return successNoop("task is already finished")
	}
	if vt.Status != evergreen.TaskUndispatched {
		return successNoop("task is already running")
	}

	// Claim the task before dequeueing it. SetCompletedBy only matches while
	// the task is undispatched, so a task that was just dispatched no-ops here.
	if err = vt.SetCompletedBy(ctx, runner.Id); err != nil {
		if adb.ResultsNotFound(err) {
			return successNoop("task is no longer waiting to be dispatched")
		}
		return failed(errors.Wrap(err, "setting completing task").Error())
	}

	if vt.Activated {
		// The task no longer needs to run on a host now that its results are
		// available.
		grip.Warning(ctx, message.WrapError(model.DequeueTask(ctx, vt.Id, vt.DistroId), message.Fields{
			"message": "dequeueing virtual task for push completion",
			"task_id": vt.Id,
			"distro":  vt.DistroId,
		}))
	}

	// A push-completed task never dispatched, so its output info must be set
	// here for the pushed results to be locatable.
	vt.TaskOutputInfo = task.InitializeTaskOutput(h.env, vt.Project)

	if tr := completion.TestResults; tr != nil {
		if err = task.AppendVirtualTestResultMetadata(ctx, vt, h.env, tr.FailedSample, tr.Stats.FailedCount, tr.Stats.TotalCount, tr.CreatedAt); err != nil {
			return failed(errors.Wrap(err, "appending test result metadata").Error())
		}
		if err = vt.SetResultsInfo(ctx, tr.Stats.FailedCount > 0); err != nil {
			return failed(errors.Wrap(err, "setting results info").Error())
		}
	}

	if len(files) > 0 {
		entry := &artifact.Entry{
			TaskId:          vt.Id,
			TaskDisplayName: vt.DisplayName,
			BuildId:         vt.BuildId,
			Execution:       vt.Execution,
			CreateTime:      time.Now(),
			Files:           artifact.EscapeFiles(files),
		}
		if err = entry.Upsert(ctx); err != nil {
			return failed(errors.Wrap(err, "attaching artifact files").Error())
		}
	}

	finishTime := time.Now()
	// A push-completed task has no runtime of its own.
	vt.StartTime = finishTime
	detail := &apimodels.TaskEndDetail{
		Status:                    completion.Status,
		ExecutionPlatform:         string(task.ExecutionPlatformVirtual),
		ExternalExecutionMetadata: completion.ExternalMetadata,
	}
	if err = model.MarkEnd(ctx, h.env.Settings(), vt, evergreen.APIServerTaskActivator, finishTime, detail); err != nil {
		return failed(errors.Wrap(err, "marking task finished").Error())
	}

	return apimodels.VirtualTaskCompletionResult{
		TaskID:  completion.TaskID,
		Outcome: apimodels.VirtualTaskCompletionOutcomeSuccess,
	}
}
