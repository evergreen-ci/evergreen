package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// resolveVirtualTaskRunner validates that virtual tasks are enabled and resolves
// the runner task and its project, checking that the caller is authorized to
// push-complete virtual tasks for it. A non-nil responder means that there's an
// error and the request should fail.
func resolveVirtualTaskRunner(ctx context.Context, taskID string) (*task.Task, gimlet.Responder) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting service flags"))
	}
	if flags.VirtualTasksDisabled {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "virtual tasks are disabled",
		})
	}

	// For task auth, the authenticated task is the runner. For user auth
	// (service users), the URL task ID identifies the runner on whose behalf
	// the user is pushing results.
	runner := GetTask(ctx)
	if runner == nil {
		runner, err = task.FindOneId(ctx, taskID)
		if err != nil {
			return nil, gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", taskID))
		}
		if runner == nil {
			return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task '%s' not found", taskID),
			})
		}
	}

	pRef, err := model.FindMergedProjectRef(ctx, runner.Project, runner.Version, false)
	if err != nil {
		return nil, gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", runner.Project))
	}
	if pRef == nil {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", runner.Project),
		})
	}
	if !pRef.IsVirtualTasksEnabled() {
		return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
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
			return nil, gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusForbidden,
				Message:    fmt.Sprintf("user '%s' does not have permission to complete virtual tasks in project '%s'", usr.Username(), pRef.Identifier),
			})
		}
	}

	return runner, nil
}

// validateVirtualTaskTarget checks that the virtual task can be push-completed
// by the runner. A non-empty reason means the completion should no-op; a
// non-nil error is a hard failure.
func validateVirtualTaskTarget(ctx context.Context, runner *task.Task, taskID string, execution int) (*task.Task, string, error) {
	vt, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return nil, "", errors.Wrap(err, "finding task")
	}
	if vt == nil {
		return nil, "", errors.New("task not found")
	}
	if !vt.IsVirtual {
		return nil, "", errors.New("task is not a virtual task")
	}
	if vt.Version != runner.Version {
		return nil, "", errors.Errorf("task does not belong to the same version as task '%s'", runner.Id)
	}
	if vt.Execution != execution {
		return nil, fmt.Sprintf("completion is for execution %d but the task is on execution %d", execution, vt.Execution), nil
	}
	if vt.IsFinished() {
		return nil, "task is already finished", nil
	}
	if vt.Status != evergreen.TaskUndispatched {
		return nil, "task is already running", nil
	}
	return vt, "", nil
}

// claimVirtualTask locks the virtual task for the runner so it is not
// dispatchable. A non-empty reason means the virtual task is already claimed by
// another runner and the completion should no-op.
func claimVirtualTask(ctx context.Context, runner *task.Task, vt *task.Task) (string, error) {
	if vt.CompletedBy != "" && vt.CompletedBy != runner.Id {
		return fmt.Sprintf("task is already being completed by task '%s'", vt.CompletedBy), nil
	}

	// Claim the task before dequeueing it. SetCompletedBy only matches while
	// the task is undispatched, so a task that was just dispatched no-ops here.
	if vt.CompletedBy == "" {
		if err := vt.SetCompletedBy(ctx, runner.Id); err != nil {
			if adb.ResultsNotFound(err) {
				return "task is no longer waiting to be dispatched or is already being push-completed by another task", nil
			}
			return "", errors.Wrap(err, "setting completing task")
		}
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
	return "", nil
}

// POST /task/{task_id}/virtual_tasks/complete
type completeVirtualTasksHandler struct {
	env    evergreen.Environment
	taskID string
	body   apimodels.CompleteVirtualTasksRequest

	// roleExternalID is the STS external ID to presign role-backed artifacts
	// with, computed from the runner so it matches the runner's AssumeRole.
	roleExternalID string
}

func makeCompleteVirtualTasks(env evergreen.Environment) gimlet.RouteHandler {
	return &completeVirtualTasksHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Push-complete virtual tasks
//	@Description	Push-completes a batch of virtual tasks with results. The task_id in the URL identifies the runner task. The route accepts task auth or service user auth (must have task admin permissions). Tasks that are already finished, already running, or whose execution number does not match will no-op.
//	@Tags			tasks
//	@Router			/task/{task_id}/virtual_tasks/complete [post]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path		string									true	"the runner task ID"
//	@Param			{object}	body		apimodels.CompleteVirtualTasksRequest	true	"virtual task completions to push"
//	@Success		201			{object}	apimodels.CompleteVirtualTasksResponse
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
	if len(h.body.Tasks) > evergreen.MaxVirtualTaskBatchSize {
		return errors.Errorf("batch size %d exceeds the maximum of %d", len(h.body.Tasks), evergreen.MaxVirtualTaskBatchSize)
	}
	return nil
}

func (h *completeVirtualTasksHandler) Run(ctx context.Context) gimlet.Responder {
	runner, errResp := resolveVirtualTaskRunner(ctx, h.taskID)
	if errResp != nil {
		return errResp
	}

	if h.needsRoleExternalID() {
		externalID, err := virtualTaskRoleExternalID(ctx, runner)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "computing external ID for role-backed artifacts"))
		}
		h.roleExternalID = externalID
	}

	response := apimodels.CompleteVirtualTasksResponse{}
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
		response.Results = append(response.Results, result)
	}

	responder := gimlet.NewJSONResponse(response)
	if err := responder.SetStatus(http.StatusCreated); err != nil {
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
		file := artifact.File{
			Name:       a.Name,
			Link:       a.URL,
			Visibility: a.Visibility,
		}
		if a.Visibility == artifact.Signed {
			if err := h.setSignedArtifactFields(&file, a); err != nil {
				return failed(err.Error())
			}
		}
		files = append(files, file)
	}
	if err := artifact.ValidateFiles(files); err != nil {
		return failed(err.Error())
	}

	vt, reason, err := validateVirtualTaskTarget(ctx, runner, completion.TaskID, completion.Execution)
	if err != nil {
		return failed(err.Error())
	}
	if reason != "" {
		return successNoop(reason)
	}

	if reason, err = claimVirtualTask(ctx, runner, vt); err != nil {
		return failed(err.Error())
	} else if reason != "" {
		return successNoop(reason)
	}

	// Task output is typically initialized when a task is dispatched, but
	// virtual tasks are never dispatched. Instead, virtual tasks share the
	// runner's task output for test logs/results.
	if runner.TaskOutputInfo == nil {
		return failed(fmt.Sprintf("runner task '%s' has not initialized its task output", runner.Id))
	}
	vt.TaskOutputInfo = runner.TaskOutputInfo

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
		ExecutionPlatform:         string(vt.ExecutionPlatform),
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

// setSignedArtifactFields fills in the S3 location and presign credentials for
// a signed artifact, which are required to presign it later. It errors if the
// fields are invalid.
func (h *completeVirtualTasksHandler) setSignedArtifactFields(file *artifact.File, a apimodels.VirtualTaskArtifact) error {
	if a.Bucket == "" || a.FileKey == "" {
		return errors.Errorf("signed artifact '%s' must specify bucket and file_key", a.Name)
	}
	awsKeyVarNameSet := a.AWSKeyVarName != ""
	awsSecretVarNameSet := a.AWSSecretVarName != ""
	if awsKeyVarNameSet != awsSecretVarNameSet {
		return errors.Errorf("artifact '%s' must specify aws_key_var_name and aws_secret_var_name together", a.Name)
	}
	if a.AWSRoleARN != "" && a.AWSKeyVarName != "" {
		return errors.Errorf("artifact '%s' cannot combine aws_role_arn with aws_key_var_name/aws_secret_var_name", a.Name)
	}

	file.Bucket = a.Bucket
	file.FileKey = a.FileKey
	file.AWSKeyVarName = a.AWSKeyVarName
	file.AWSSecretVarName = a.AWSSecretVarName
	file.AWSRoleARN = a.AWSRoleARN

	if a.PresignDuration != "" {
		duration, err := time.ParseDuration(a.PresignDuration)
		if err != nil {
			return errors.Wrapf(err, "parsing presign_duration '%s' for artifact '%s'", a.PresignDuration, a.Name)
		}
		if err := artifact.ValidatePresignDuration(duration); err != nil {
			return errors.Wrapf(err, "validating presign_duration for artifact '%s'", a.Name)
		}
		file.PresignDuration = duration
	}

	if file.AWSRoleARN != "" {
		if file.PresignDuration != 0 {
			return errors.Errorf("artifact '%s' cannot combine aws_role_arn with presign_duration", a.Name)
		}
		file.ExternalID = h.roleExternalID
	}

	return nil
}

// needsRoleExternalID reports whether any pushed artifact uses a role ARN to
// presign, in which case the external ID must be computed from the runner.
func (h *completeVirtualTasksHandler) needsRoleExternalID() bool {
	for _, completion := range h.body.Tasks {
		for _, a := range completion.Artifacts {
			if a.Visibility == artifact.Signed && a.AWSRoleARN != "" {
				return true
			}
		}
	}
	return false
}

// virtualTaskRoleExternalID returns the STS external ID the runner uses when
// assuming roles, so role-backed artifacts are presigned with the same external
// ID the runner used to upload them.
func virtualTaskRoleExternalID(ctx context.Context, runner *task.Task) (string, error) {
	pRef, err := model.GetProjectRefForTask(ctx, runner.Id)
	if err != nil {
		return "", errors.Wrapf(err, "getting project ref for runner task '%s'", runner.Id)
	}

	var isDebug bool
	if runner.HostId != "" {
		runnerHost, err := host.FindOneId(ctx, runner.HostId)
		if err != nil {
			return "", errors.Wrapf(err, "finding host '%s' for runner task '%s'", runner.HostId, runner.Id)
		}
		isDebug = runnerHost != nil && runnerHost.IsDebug
	}

	return cloud.CreateExternalID(runner, pRef, isDebug), nil
}

// POST /task/{task_id}/virtual_tasks/prepare
type prepareVirtualTasksHandler struct {
	env    evergreen.Environment
	taskID string
	body   apimodels.PrepareVirtualTasksRequest
}

func makePrepareVirtualTasks(env evergreen.Environment) gimlet.RouteHandler {
	return &prepareVirtualTasksHandler{env: env}
}

func (h *prepareVirtualTasksHandler) Factory() gimlet.RouteHandler {
	return &prepareVirtualTasksHandler{env: h.env}
}

func (h *prepareVirtualTasksHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.body); err != nil {
		return errors.Wrapf(err, "reading virtual task preparations for task '%s'", h.taskID)
	}
	if len(h.body.Tasks) == 0 {
		return errors.New("must specify at least one task to prepare")
	}
	if len(h.body.Tasks) > evergreen.MaxVirtualTaskBatchSize {
		return errors.Errorf("batch size %d exceeds the maximum of %d", len(h.body.Tasks), evergreen.MaxVirtualTaskBatchSize)
	}
	return nil
}

func (h *prepareVirtualTasksHandler) Run(ctx context.Context) gimlet.Responder {
	runner, errResp := resolveVirtualTaskRunner(ctx, h.taskID)
	if errResp != nil {
		return errResp
	}

	response := apimodels.PrepareVirtualTasksResponse{}
	for _, prep := range h.body.Tasks {
		result := h.prepareTask(ctx, runner, prep)
		grip.Info(ctx, message.Fields{
			"message":   "virtual task preparation",
			"runner":    runner.Id,
			"task_id":   prep.TaskID,
			"outcome":   result.Outcome,
			"reason":    result.Reason,
			"execution": prep.Execution,
		})
		response.Results = append(response.Results, result)
	}

	return gimlet.NewJSONResponse(response)
}

// prepareTask validates and locks a single virtual task so the runner can
// safely upload its test results before push-completing it. This prevents the
// virtual task from potentially being dispatched.
func (h *prepareVirtualTasksHandler) prepareTask(ctx context.Context, runner *task.Task, prep apimodels.VirtualTaskPreparation) apimodels.VirtualTaskPreparationResult {
	failed := func(reason string) apimodels.VirtualTaskPreparationResult {
		return apimodels.VirtualTaskPreparationResult{
			TaskID:  prep.TaskID,
			Outcome: apimodels.VirtualTaskCompletionOutcomeFailed,
			Reason:  reason,
		}
	}
	// No-ops return success so that runners don't retry idempotent pushes.
	successNoop := func(reason string) apimodels.VirtualTaskPreparationResult {
		return apimodels.VirtualTaskPreparationResult{
			TaskID:  prep.TaskID,
			Outcome: apimodels.VirtualTaskCompletionOutcomeSuccess,
			Reason:  reason,
		}
	}

	vt, reason, err := validateVirtualTaskTarget(ctx, runner, prep.TaskID, prep.Execution)
	if err != nil {
		return failed(err.Error())
	}
	if reason != "" {
		return successNoop(reason)
	}

	if reason, err = claimVirtualTask(ctx, runner, vt); err != nil {
		return failed(err.Error())
	} else if reason != "" {
		return successNoop(reason)
	}

	info, err := task.MakeTestResultsInfo(ctx, vt)
	if err != nil {
		return failed(errors.Wrap(err, "making test results info").Error())
	}

	return apimodels.VirtualTaskPreparationResult{
		TaskID:          prep.TaskID,
		Outcome:         apimodels.VirtualTaskCompletionOutcomeSuccess,
		TestResultsInfo: &info,
		TaskCreateTime:  vt.CreateTime,
	}
}
