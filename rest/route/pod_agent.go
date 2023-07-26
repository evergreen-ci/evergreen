package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

/////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/provisioning_script

type podProvisioningScript struct {
	settings *evergreen.Settings
	podID    string
}

func makePodProvisioningScript(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podProvisioningScript{settings: settings}
}

func (h *podProvisioningScript) Factory() gimlet.RouteHandler {
	return &podProvisioningScript{settings: h.settings}
}

func (h *podProvisioningScript) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	return nil
}

// Run returns the script to provision this pod. Unlike the typical REST routes
// that return JSON responses, this returns text responses for simplicity and
// because the pod's containers unlikely to be equipped with the tooling to
// parse JSON output.
func (h *podProvisioningScript) Run(ctx context.Context) gimlet.Responder {
	p, err := pod.FindOneByID(h.podID)
	if err != nil {
		return gimlet.NewTextInternalErrorResponse(errors.Wrap(err, "finding pod"))
	}
	if p == nil {
		return gimlet.NewTextErrorResponse(errors.Errorf("pod '%s' not found", h.podID))
	}

	grip.Info(message.Fields{
		"message":                   "pod requested its provisioning script",
		"pod":                       p.ID,
		"external_id":               p.Resources.ExternalID,
		"secs_since_pod_allocation": time.Since(p.TimeInfo.Initializing).Seconds(),
		"secs_since_pod_creation":   time.Since(p.TimeInfo.Starting).Seconds(),
	})

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.NewTextInternalErrorResponse(errors.Wrap(err, "getting service flags"))
	}

	script := h.agentScript(p, !flags.S3BinaryDownloadsDisabled)

	return gimlet.NewTextResponse(script)
}

// agentScript returns the script to provision and run the agent in the pod's
// container. On Linux, this is a shell script. On Windows, this is a cmd.exe
// batch script.
func (h *podProvisioningScript) agentScript(p *pod.Pod, downloadFromS3 bool) string {
	scriptCmds := []string{h.downloadAgentCommands(p, downloadFromS3)}
	if p.TaskContainerCreationOpts.OS == pod.OSLinux {
		scriptCmds = append(scriptCmds, fmt.Sprintf("chmod +x %s", h.clientName(p)))
	}
	agentCmd := strings.Join(h.agentCommand(p), " ")
	scriptCmds = append(scriptCmds, agentCmd)

	if p.TaskContainerCreationOpts.OS == pod.OSLinux {
		return strings.Join(scriptCmds, " && ")
	}

	// This chains together the PowerShell commands so that they run in order,
	// but they also run regardless of whether the previous command succeeded,
	// which is undesirable. It would be preferable to use pipeline chaining
	// operators instead (like bash's && and || operators) but they were not
	// introduced until PowerShell 7. Users may not provide a sufficiently
	// up-to-date version of PowerShell on their images to use these operators.
	// Docs: https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_pipeline_chain_operators
	return strings.Join(scriptCmds, "; ")
}

// agentCommand returns the arguments to start the agent in the pod's container.
func (h *podProvisioningScript) agentCommand(p *pod.Pod) []string {
	var pathSep string
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		pathSep = "\\"
	} else {
		pathSep = "/"
	}

	return []string{
		fmt.Sprintf(".%s%s", pathSep, h.clientName(p)),
		"agent",
		fmt.Sprintf("--api_server=%s", h.settings.ApiUrl),
		"--mode=pod",
		"--log_output=file",
		fmt.Sprintf("--log_prefix=%s", strings.Join([]string{p.TaskContainerCreationOpts.WorkingDir, "agent"}, "/")),
		fmt.Sprintf("--working_directory=%s", p.TaskContainerCreationOpts.WorkingDir),
	}
}

// downloadAgentCommands returns the commands to download the agent in the pod's
// container.
func (h *podProvisioningScript) downloadAgentCommands(p *pod.Pod, downloadFromS3 bool) string {
	const (
		curlDefaultNumRetries = 10
		curlDefaultMaxSecs    = 100
	)
	retryArgs := h.curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	curlExecutable := "curl"
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		curlExecutable = curlExecutable + ".exe"
	}

	if downloadFromS3 && h.settings.PodLifecycle.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading
		// from the app server if it fails.
		// Include -f to return an error code from curl if the HTTP request
		// fails (e.g. it receives 403 Forbidden or 404 Not Found).

		if p.TaskContainerCreationOpts.OS == pod.OSWindows {
			// PowerShell supports pipeline chaining operators (like bash's ||
			// operator) as of PowerShell 7. However, users may not provide a
			// sufficiently up-to-date version of PowerShell on their images to
			// use these operators. Therefore, use a PowerShell if-else
			// statement to produce the equivalent functionality.
			// Docs: https://docs.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_pipeline_chain_operators
			return fmt.Sprintf("if (%s -fLO %s %s) {} else { %s -fLO %s %s }", curlExecutable, h.s3ClientURL(p), retryArgs, curlExecutable, h.evergreenClientURL(p), retryArgs)
		}

		return fmt.Sprintf("(%s -fLO %s %s || %s -fLO %s %s)", curlExecutable, h.s3ClientURL(p), retryArgs, curlExecutable, h.evergreenClientURL(p), retryArgs)

	}

	return fmt.Sprintf("%s -fLO %s %s", curlExecutable, h.evergreenClientURL(p), retryArgs)
}

// evergreenClientURL returns the URL used to get the latest Evergreen client
// version directly from the Evergreen server.
func (h *podProvisioningScript) evergreenClientURL(p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(h.settings.ApiUrl, "/"),
		strings.TrimSuffix(h.settings.ClientBinariesDir, "/"),
		h.clientURLSubpath(p),
	}, "/")
}

// s3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func (h *podProvisioningScript) s3ClientURL(p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(h.settings.PodLifecycle.S3BaseURL, "/"),
		evergreen.BuildRevision,
		h.clientURLSubpath(p),
	}, "/")
}

// clientURLSubpath returns the URL path to the compiled agent.
func (h *podProvisioningScript) clientURLSubpath(p *pod.Pod) string {
	return strings.Join([]string{
		fmt.Sprintf("%s_%s", p.TaskContainerCreationOpts.OS, p.TaskContainerCreationOpts.Arch),
		h.clientName(p),
	}, "/")
}

// clientName returns the file name of the agent binary.
func (h *podProvisioningScript) clientName(p *pod.Pod) string {
	name := "evergreen"
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		return name + ".exe"
	}
	return name
}

// curlRetryArgs constructs options to configure the curl retry behavior.
func (h *podProvisioningScript) curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}

// GET /rest/v2/pods/{pod_id}/agent/next_task

type podAgentNextTask struct {
	env   evergreen.Environment
	podID string
}

func makePodAgentNextTask(env evergreen.Environment) gimlet.RouteHandler {
	return &podAgentNextTask{
		env: env,
	}
}

func (h *podAgentNextTask) Factory() gimlet.RouteHandler {
	return &podAgentNextTask{
		env: h.env,
	}
}

func (h *podAgentNextTask) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	if h.podID == "" {
		return errors.New("missing pod ID")
	}
	return nil
}

func (h *podAgentNextTask) Run(ctx context.Context) gimlet.Responder {
	p, err := data.FindPodByID(h.podID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if err := h.transitionStartingToRunning(p); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "marking pod as running"))
	}

	h.setAgentFirstContactTime(p)

	if p.Status == pod.StatusTerminated || p.Status == pod.StatusDecommissioned {
		if err = h.prepareForPodTermination(ctx, p, "pod is no longer running"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
		return gimlet.NewJSONResponse(&apimodels.NextTaskResponse{})
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving admin settings"))
	}
	if flags.TaskDispatchDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), "task dispatch is disabled, returning no task")
		if err = h.prepareForPodTermination(ctx, p, "task dispatching is disabled"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
		return gimlet.NewJSONResponse(&apimodels.NextTaskResponse{})
	}

	if p.TaskRuntimeInfo.RunningTaskID != "" {
		if resp := h.checkAndRedispatchRunningTask(ctx, p); resp != nil {
			return resp
		}
	}

	pd, err := h.findDispatcher()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	nextTask, err := pd.AssignNextTask(ctx, h.env, p)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "dispatching next task"))
	}
	if nextTask == nil {
		if err = h.prepareForPodTermination(ctx, p, "dispatch queue has no more tasks to run"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
		return gimlet.NewJSONResponse(&apimodels.NextTaskResponse{})
	}

	return gimlet.NewJSONResponse(&apimodels.NextTaskResponse{
		TaskId:     nextTask.Id,
		TaskSecret: nextTask.Secret,
		TaskGroup:  nextTask.TaskGroup,
		Version:    nextTask.Version,
		Build:      nextTask.BuildId,
	})
}

// prepareForPodTermination will mark the pod as preparing to terminate if it is
// not already attempting to terminate and enqueue the job to terminate it with
// a detailed reason for doing so.
func (h *podAgentNextTask) prepareForPodTermination(ctx context.Context, p *pod.Pod, reason string) error {
	if p.Status != pod.StatusDecommissioned && p.Status != pod.StatusTerminated {
		if err := p.UpdateStatus(pod.StatusDecommissioned, reason); err != nil {
			return errors.Wrap(err, "updating pod status to decommissioned")
		}
	}

	j := units.NewPodTerminationJob(h.podID, reason, utility.RoundPartOfMinute(0))
	if err := amboy.EnqueueUniqueJob(ctx, h.env.RemoteQueue(), j); err != nil {
		return errors.Wrap(err, "enqueueing job to terminate pod")
	}
	return nil
}

// transitionStartingToRunning transitions the pod that is still starting up to
// indicate that it is running and ready to accept tasks.
func (h *podAgentNextTask) transitionStartingToRunning(p *pod.Pod) error {
	if p.Status != pod.StatusStarting {
		return nil
	}

	return p.UpdateStatus(pod.StatusRunning, "agent requested next task to run, indicating that it is now running")
}

func (h *podAgentNextTask) setAgentFirstContactTime(p *pod.Pod) {
	if p.TimeInfo.Initializing.IsZero() {
		return
	}

	if err := p.UpdateAgentStartTime(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update pod's agent first contact time",
			"pod":     p.ID,
		}))
		return
	}

	grip.Info(message.Fields{
		"message":                   "pod initiated first contact with application server",
		"included_on":               evergreen.ContainerHealthDashboard,
		"pod":                       p.ID,
		"secs_since_pod_allocation": time.Since(p.TimeInfo.Initializing).Seconds(),
		"secs_since_pod_creation":   time.Since(p.TimeInfo.Starting).Seconds(),
	})
}

func (h *podAgentNextTask) findDispatcher() (*dispatcher.PodDispatcher, error) {
	pd, err := dispatcher.FindOneByPodID(h.podID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding dispatcher").Error(),
		}
	}
	if pd == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("dispatcher for pod '%s' not found", h.podID),
		}
	}

	return pd, nil
}

func (h *podAgentNextTask) checkAndRedispatchRunningTask(ctx context.Context, p *pod.Pod) gimlet.Responder {
	t, err := task.FindOneIdAndExecution(p.TaskRuntimeInfo.RunningTaskID, p.TaskRuntimeInfo.RunningTaskExecution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting running task '%s' execution %d", p.TaskRuntimeInfo.RunningTaskID, p.TaskRuntimeInfo.RunningTaskExecution))
	}
	if t == nil {
		if err := h.cleanUpPodAfterRedispatchFailure(ctx, p, "", "the task does not exist"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "cleaning up after attempting to re-dispatch a nonexistent task"))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}

	if t.Archived {
		if err := h.cleanUpPodAfterRedispatchFailure(ctx, p, t.Status, "the task is archived"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "cleaning up after attempting to re-dispatch an archived task"))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}
	if t.IsFinished() {
		if err := h.cleanUpPodAfterRedispatchFailure(ctx, p, t.Status, "the task is already finished"); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "cleaning up after attempting to re-dispatch an already-finished task"))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}

	if t.IsPartOfDisplay() {
		if err = model.UpdateDisplayTaskForTask(t); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating parent display task for task '%s'", t.Id))
		}
	}

	return gimlet.NewJSONResponse(&apimodels.NextTaskResponse{
		TaskId:     t.Id,
		TaskSecret: t.Secret,
		TaskGroup:  t.TaskGroup,
		Version:    t.Version,
		Build:      t.BuildId,
	})
}

// cleanUpPodWithUndispatchableAssignedTask cleans up the state of a pod that
// has a task assigned to run but is unable to re-dispatch it.
func (h *podAgentNextTask) cleanUpPodAfterRedispatchFailure(ctx context.Context, p *pod.Pod, taskStatus string, reason string) error {
	taskID := p.TaskRuntimeInfo.RunningTaskID
	taskExecution := p.TaskRuntimeInfo.RunningTaskExecution
	reason = fmt.Sprintf("cannot re-dispatch task '%s' execution %d because %s", taskID, taskExecution, reason)

	grip.Warning(message.Fields{
		"message":   "clearing pod assigned task and preparing for pod termination due to task that cannot be re-dispatched",
		"reason":    reason,
		"task":      taskID,
		"execution": taskExecution,
		"status":    taskStatus,
	})

	if err := p.ClearRunningTask(); err != nil {
		return errors.Wrapf(err, "clearing task '%s' execution %d assigned to pod", taskID, taskExecution)
	}
	if err := h.prepareForPodTermination(ctx, p, reason); err != nil {
		return err
	}
	return nil
}

// POST /rest/v2/pods/{pod_id}/task/{task_id}/end

type podAgentEndTask struct {
	env     evergreen.Environment
	podID   string
	taskID  string
	details apimodels.TaskEndDetail
}

func makePodAgentEndTask(env evergreen.Environment) gimlet.RouteHandler {
	return &podAgentEndTask{
		env: env,
	}
}

func (h *podAgentEndTask) Factory() gimlet.RouteHandler {
	return &podAgentEndTask{
		env: h.env,
	}
}

func (h *podAgentEndTask) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	if h.podID == "" {
		return errors.New("missing pod ID")
	}
	h.taskID = gimlet.GetVars(r)["task_id"]
	if h.taskID == "" {
		return errors.New("missing task ID")
	}
	details := apimodels.TaskEndDetail{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), &details); err != nil {
		return errors.Wrap(err, "reading task end details from JSON request body")
	}
	h.details = details

	// Check that status is either finished or aborted (i.e. undispatched)
	if !evergreen.IsValidTaskEndStatus(details.Status) && details.Status != evergreen.TaskUndispatched {
		return errors.Errorf("invalid end status '%s' for task '%s'", details.Status, h.taskID)
	}
	return nil
}

// Run finishes the given task and generates a job to collect task end stats.
// It then marks the task as finished. If the task is aborted, this will no-op.
func (h *podAgentEndTask) Run(ctx context.Context) gimlet.Responder {
	finishTime := time.Now()
	p, err := data.FindPodByID(h.podID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	t, err := data.FindTask(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	endTaskResp := &apimodels.EndTaskResponse{}

	if p.TaskRuntimeInfo.RunningTaskID == "" {
		grip.Notice(message.Fields{
			"message":                 "pod is not assigned task, agent should move onto requesting next task",
			"task_id":                 t.Id,
			"task_status_from_db":     t.Status,
			"task_details_from_db":    t.Details,
			"current_agent":           p.AgentVersion == evergreen.AgentVersion,
			"agent_version":           p.AgentVersion,
			"build_revision":          evergreen.BuildRevision,
			"build_agent":             evergreen.AgentVersion,
			"task_details_from_agent": h.details,
			"pod_id":                  p.ID,
		})
		return gimlet.NewJSONResponse(endTaskResp)
	}

	projectRef, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s' for version '%s'", t.Project, t.Version))
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Errorf("project '%s' not found", t.Project).Error(),
		})
	}

	// The order of operations here for clearing the task from the pod and
	// marking the task finished is critical and must be done in this particular
	// order. See the host end task route for more detailed explanation.

	// Clear the running task on the pod now that the task has finished.
	if err = p.ClearRunningTask(); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "clearing running task '%s' for pod '%s'", t.Id, p.ID))
	}

	deactivatePrevious := utility.FromBoolPtr(projectRef.DeactivatePrevious)
	err = model.MarkEnd(ctx, h.env.Settings(), t, evergreen.APIServerTaskActivator, finishTime, &h.details, deactivatePrevious)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "calling mark finish on task '%s'", t.Id))
	}

	if evergreen.IsCommitQueueRequester(t.Requester) {
		if err = model.HandleEndTaskForCommitQueueTask(ctx, t, h.details.Status); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// the task was aborted if it is still in undispatched.
	// the active state should be inactive.
	if h.details.Status == evergreen.TaskUndispatched {
		if t.Activated {
			grip.Warning(message.Fields{
				"message": fmt.Sprintf("task '%s' is active and undispatched after being marked as finished", t.Id),
				"task_id": t.Id,
				"path":    fmt.Sprintf("/rest/v2/pods/%s/task/%s/end", p.ID, t.Id),
			})
			return gimlet.NewJSONResponse(&apimodels.EndTaskResponse{})
		}
		grip.Info(message.Fields{
			"message": fmt.Sprintf("task '%s' has been aborted", t.Id),
			"task_id": t.Id,
			"path":    fmt.Sprintf("/rest/v2/pods/%s/task/%s/end", p.ID, t.Id),
		})
		return gimlet.NewJSONResponse(&apimodels.EndTaskResponse{})
	}

	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "getting display task"))
	}

	if p.Status != pod.StatusRunning {
		j := units.NewPodTerminationJob(h.podID, "pod is no longer running", utility.RoundPartOfMinute(0))
		if err := amboy.EnqueueUniqueJob(ctx, h.env.RemoteQueue(), j); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "enqueueing job to terminate pod"))
		}
	}

	msg := message.Fields{
		"message":     "Successfully marked task as finished",
		"task_id":     t.Id,
		"execution":   t.Execution,
		"operation":   "mark end",
		"duration":    time.Since(finishTime),
		"should_exit": endTaskResp.ShouldExit,
		"status":      t.Status,
		"path":        fmt.Sprintf("/rest/v2/pods/%s/task/%s/end", p.ID, t.Id),
	}

	if t.IsPartOfDisplay() {
		msg["display_task_id"] = t.DisplayTaskId
	}

	grip.Info(msg)
	return gimlet.NewJSONResponse(endTaskResp)
}
