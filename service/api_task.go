package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// if a host encounters more than this number of system failures, then it should be disabled.
const consecutiveSystemFailureThreshold = 3
const taskDispatcherTTL = time.Minute

// StartTask is the handler function that retrieves the task from the request
// and acquires the global lock
// With the lock, it marks associated tasks, builds, and versions as started.
// It then updates the host document with relevant information, including the pid
// of the agent, and ensures that the host has the running task field set.
func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	var err error

	t := MustHaveTask(r)
	grip.Debug(message.Fields{
		"message": "marking task started",
		"task_id": t.Id,
		"details": t.Details,
	})

	taskStartInfo := &apimodels.TaskStartRequest{}
	if err = utility.ReadJSON(utility.NewRequestReader(r), taskStartInfo); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrapf(err, "Error reading task start request for %s", t.Id))
		return
	}

	updates := model.StatusChanges{}
	if err = model.MarkStart(t, &updates); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "Error marking task '%s' started", t.Id))
		return
	}

	if len(updates.PatchNewStatus) != 0 {
		event.LogPatchStateChangeEvent(t.Version, updates.PatchNewStatus)
	}
	if len(updates.BuildNewStatus) != 0 {
		event.LogBuildStateChangeEvent(t.BuildId, updates.BuildNewStatus)
	}

	h, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		message := errors.Wrapf(err, "Error finding host running task %s", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if h == nil {
		message := errors.Errorf("No host found running task %v", t.Id)
		if t.HostId != "" {
			message = errors.Errorf("No host found running task %s but task is said to be running on %s",
				t.Id, t.HostId)
		}

		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	idleTimeStartAt := h.LastTaskCompletedTime
	if idleTimeStartAt.IsZero() || idleTimeStartAt == utility.ZeroTime {
		idleTimeStartAt = h.StartTime
	}

	msg := fmt.Sprintf("Task %v started on host %v", t.Id, h.Id)

	if h.Distro.IsEphemeral() {
		job := units.NewCollectHostIdleDataJob(h, t, idleTimeStartAt, t.StartTime)
		if err = as.queue.Put(r.Context(), job); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error queuing host idle stats for %s", msg))
			return
		}
	}

	gimlet.WriteJSON(w, msg)
}

// checkHostHealth checks that host is running.
func checkHostHealth(h *host.Host) bool {
	if h.Status == evergreen.HostRunning {
		return false
	}

	// User data can start anytime after the instance is created, so the app
	// server may not have marked it as running yet.
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData && h.Status == evergreen.HostStarting {
		return false
	}

	grip.Info(message.Fields{
		"message":                 "host is not running, so agent should exit",
		"status":                  h.Status,
		"bootstrap_method":        h.Distro.BootstrapSettings.Method,
		"bootstrap_communication": h.Distro.BootstrapSettings.Communication,
		"host_id":                 h.Id,
	})

	return true
}

// agentRevisionIsOld checks that the agent revision is current.
func agentRevisionIsOld(h *host.Host) bool {
	if h.AgentRevision != evergreen.AgentVersion {
		grip.InfoWhen(h.Distro.LegacyBootstrap(), message.Fields{
			"message":       "agent has wrong revision, so it should exit",
			"host_revision": h.AgentRevision,
			"build":         evergreen.BuildRevision,
			"agent_version": evergreen.AgentVersion,
		})
		return true
	}
	return false
}

// EndTask creates test results from the request and the project config.
// It then acquires the lock, and with it, marks tasks as finished or inactive if aborted.
// If the task is a patch, it will alert the users based on failures
// It also updates the expected task duration of the task for scheduling.
func (as *APIServer) EndTask(w http.ResponseWriter, r *http.Request) {
	finishTime := time.Now()

	t := MustHaveTask(r)
	currentHost := MustHaveHost(r)

	details := &apimodels.TaskEndDetail{}
	endTaskResp := &apimodels.EndTaskResponse{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that status is either finished or aborted (i.e. undispatched)
	if !evergreen.IsValidTaskEndStatus(details.Status) && details.Status != evergreen.TaskUndispatched {
		msg := fmt.Errorf("invalid end status '%s' for task %s", details.Status, t.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	if currentHost.RunningTask == "" {
		grip.Notice(message.Fields{
			"message":                 "host is not assigned task, not clearing, asking agent to exit",
			"task_id":                 t.Id,
			"task_status_from_db":     t.Status,
			"task_details_from_db":    t.Details,
			"current_agent":           currentHost.AgentRevision == evergreen.AgentVersion,
			"agent_version":           currentHost.AgentRevision,
			"build_revision":          evergreen.BuildRevision,
			"build_agent":             evergreen.AgentVersion,
			"task_details_from_agent": details,
			"host_id":                 currentHost.Id,
			"distro":                  currentHost.Distro.Id,
		})
		endTaskResp.ShouldExit = true
		gimlet.WriteJSON(w, endTaskResp)
		return
	}

	model.CheckAndBlockSingleHostTaskGroup(t, details.Status)

	projectRef, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("empty projectRef for task"))
		return
	}

	// mark task as finished
	deactivatePrevious := utility.FromBoolPtr(projectRef.DeactivatePrevious)
	err = model.MarkEnd(t, APIServerLockTitle, finishTime, details, deactivatePrevious)
	if err != nil {
		err = errors.Wrapf(err, "Error calling mark finish on task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// Clear the running task on the host now that the task has finished.
	if err := currentHost.ClearRunningAndSetLastTask(t); err != nil {
		err = errors.Wrapf(err, "error clearing running task %s for host %s", t.Id, currentHost.Id)
		grip.Errorf(err.Error())
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if t.Requester == evergreen.MergeTestRequester {
		if err = model.HandleEndTaskForCommitQueueTask(t, details.Status); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	// the task was aborted if it is still in undispatched.
	// the active state should be inactive.
	if details.Status == evergreen.TaskUndispatched {
		if t.Activated {
			grip.Warningf("task %v is active and undispatched after being marked as finished", t.Id)
			return
		}
		abortMsg := fmt.Sprintf("task %v has been aborted and will not run", t.Id)
		grip.Infof(abortMsg)
		endTaskResp = &apimodels.EndTaskResponse{}
		gimlet.WriteJSON(w, endTaskResp)
		return
	}

	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	job := units.NewCollectTaskEndDataJob(*t, currentHost, nil, currentHost.Id)
	if err = as.queue.Put(r.Context(), job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "couldn't queue job to update task stats accounting"))
		return
	}

	if checkHostHealth(currentHost) {
		if _, err := as.prepareHostForAgentExit(r.Context(), agentExitParams{
			host:       currentHost,
			remoteAddr: r.RemoteAddr,
		}); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "could not prepare host for agent to exit",
				"host_id":       currentHost.Id,
				"operation":     "end_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": currentHost.AgentRevision,
			}))
		}
		endTaskResp.ShouldExit = true
	}

	// we should disable hosts and prevent them from performing
	// more work if they appear to be in a bad state
	// (e.g. encountered 5 consecutive system failures)
	if event.AllRecentHostEventsMatchStatus(currentHost.Id, consecutiveSystemFailureThreshold, evergreen.TaskSystemFailed) {
		msg := "host encountered consecutive system failures"
		if currentHost.Provider != evergreen.ProviderNameStatic {
			grip.Error(message.WrapError(units.HandlePoisonedHost(r.Context(), as.env, currentHost, msg), message.Fields{
				"message": "unable to disable poisoned host",
				"host":    currentHost.Id,
			}))
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, as.env); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor",
				"host_id":       currentHost.Id,
				"operation":     "end_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": currentHost.AgentRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
		endTaskResp.ShouldExit = true
	}

	msg := message.Fields{
		"message":     "Successfully marked task as finished",
		"task_id":     t.Id,
		"execution":   t.Execution,
		"operation":   "mark end",
		"duration":    time.Since(finishTime),
		"should_exit": endTaskResp.ShouldExit,
		"status":      details.Status,
		"path":        fmt.Sprintf("/api/2/task/%s/end", t.Id),
	}

	if t.IsPartOfDisplay() {
		msg["display_task_id"] = t.DisplayTaskId
	}

	grip.Info(msg)
	gimlet.WriteJSON(w, endTaskResp)
}

// fixIntentHostRunningAgent handles an exceptional case in which an ephemeral
// host is still believed to be an intent host but somehow the agent is running
// on an EC2 instance associated with that intent host.
func (as *APIServer) fixIntentHostRunningAgent(ctx context.Context, h *host.Host, instanceID string) error {
	if !cloud.IsEc2Provider(h.Provider) {
		// Intent host issues only affect ephemeral (i.e. EC2) hosts.
		return nil
	}
	if cloud.IsEC2InstanceID(h.Id) {
		// If the host already has an instance ID, it's not an intent host.
		return nil
	}
	if instanceID == "" {
		// If the host is an intent host but the agent does not send the EC2
		// instance ID, there's nothing that can be done to fix it here.
		grip.Warning(message.Fields{
			"message":     "intent host has started an agent, but the agent did not provide an instance ID for the real host",
			"host_id":     h.Id,
			"host_status": h.Status,
			"provider":    h.Provider,
			"distro":      h.Distro.Id,
		})
		return nil
	}

	switch h.Status {
	case evergreen.HostBuilding:
		return errors.Wrap(as.transitionIntentHostToStarting(ctx, h, instanceID), "starting intent host that actually succeeded")
	case evergreen.HostBuildingFailed, evergreen.HostTerminated:
		return errors.Wrap(as.transitionIntentHostToDecommissioned(ctx, h, instanceID), "decommissioning intent host")
	default:
		return errors.Errorf("logical error: intent host is in state '%s', which should be impossible when the agent is running", h.Status)
	}
}

// transitionIntentHostToStarting converts an intent host to a real host
// because it's alive in the cloud. It is marked as starting to indicate that
// the host has started and can run tasks.
func (as *APIServer) transitionIntentHostToStarting(ctx context.Context, h *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - found EC2 instance running an agent, but Evergreen believes the host still an intent host",
		"host_id":     h.Id,
		"host_tag":    h.Tag,
		"distro":      h.Distro.Id,
		"instance_id": instanceID,
		"host_status": h.Status,
	})

	intentHostID := h.Id
	h.Id = instanceID
	h.Status = evergreen.HostStarting
	if err := host.UnsafeReplace(ctx, as.env, intentHostID, h); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStartSucceeded(h.Id)

	return nil
}

// transitionIntentHostToDecommissioned converts an intent host to a real
// host because it's alive in the cloud. It is marked as decommissioned to
// indicate that the host is not valid and should be terminated.
func (as *APIServer) transitionIntentHostToDecommissioned(ctx context.Context, h *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - found EC2 instance running an agent, but Evergreen believes the host is a stale building intent host",
		"host_id":     h.Id,
		"instance_id": instanceID,
		"host_status": h.Status,
	})

	intentHostID := h.Id
	h.Id = instanceID
	oldStatus := h.Status
	h.Status = evergreen.HostDecommissioned
	if err := host.UnsafeReplace(ctx, as.env, intentHostID, h); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStatusChanged(h.Id, oldStatus, h.Status, evergreen.User, "host started agent but intent host is already considered a failure")
	grip.Info(message.Fields{
		"message":    "intent host decommissioned",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": oldStatus,
	})

	return nil
}

type agentExitParams struct {
	host          *host.Host
	remoteAddr    string
	ec2InstanceID string
}

// prepareHostForAgentExit prepares a host to stop running tasks on the host.
// For a quarantined host, it shuts down the agent and agent monitor to prevent
// it from running further tasks. This is especially important for quarantining
// hosts, as the host may continue running, but it must stop all agent-related
// activity for now. For a terminated host, the host should already have been
// terminated but is nonetheless alive, so terminate it again.
func (as *APIServer) prepareHostForAgentExit(ctx context.Context, params agentExitParams) (shouldExit bool, err error) {
	switch params.host.Status {
	case evergreen.HostQuarantined:
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := params.host.StopAgentMonitor(ctx, as.env); err != nil {
			return true, errors.Wrap(err, "stopping agent monitor")
		}

		if err := params.host.SetNeedsAgentDeploy(true); err != nil {
			return true, errors.Wrap(err, "marking host as needing agent or agent monitor deploy")
		}

		return true, nil
	case evergreen.HostTerminated:
		// The host should already be terminated but somehow the agent is still
		// alive in the cloud. If the host was terminated very recently, it may
		// simply need time for the cloud instance to actually be terminated and
		// fully cleaned up. If this message logs, that means there is a
		// mismatch between either:
		// 1. What Evergreen believes is this host's state (terminated) and the
		//    cloud instance's actual state (not terminated). This is a bug.
		// 2. The cloud instance's status and the actual liveliness of the host.
		//    There are some cases where the agent is still checking in for a
		//    long time after the cloud provider says the instance is already
		//    terminated. There's no bug on our side, so this log is harmless.
		if time.Since(params.host.TerminationTime) > 10*time.Minute {
			msg := message.Fields{
				"message":    "DB-cloud state mismatch - host has been marked terminated in the DB but the host's agent is still running",
				"host_id":    params.host.Id,
				"distro":     params.host.Distro.Id,
				"remote":     params.remoteAddr,
				"request_id": gimlet.GetRequestID(ctx),
			}
			if cloud.IsEc2Provider(params.host.Distro.Provider) {
				msg["ec2_instance_id"] = params.ec2InstanceID
			}
			grip.Warning(msg)
		}
		return true, nil
	case evergreen.HostDecommissioned:
		return true, nil
	default:
		return false, nil
	}
}
