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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

// if a host encounters more than this number of system failures, then it should be disabled.
const consecutiveSystemFailureThreshold = 3

// GET /rest/v2/hosts/{host_id}/agent/next_task

type hostAgentNextTask struct {
	host                *host.Host
	env                 evergreen.Environment
	taskDispatcher      model.TaskQueueItemDispatcher
	taskAliasDispatcher model.TaskQueueItemDispatcher
	hostID              string
	remoteAddr          string
	details             *apimodels.GetNextTaskDetails
}

func makeHostAgentNextTask(env evergreen.Environment, taskDispatcher model.TaskQueueItemDispatcher, taskAliasDispatcher model.TaskQueueItemDispatcher) gimlet.RouteHandler {
	return &hostAgentNextTask{
		env:                 env,
		taskDispatcher:      taskDispatcher,
		taskAliasDispatcher: taskAliasDispatcher,
	}
}

func (h *hostAgentNextTask) Factory() gimlet.RouteHandler {
	return &hostAgentNextTask{
		env:                 h.env,
		taskDispatcher:      h.taskDispatcher,
		taskAliasDispatcher: h.taskAliasDispatcher,
	}
}

func (h *hostAgentNextTask) Parse(ctx context.Context, r *http.Request) error {
	h.remoteAddr = r.RemoteAddr
	if h.hostID = gimlet.GetVars(r)["host_id"]; h.hostID == "" {
		return errors.New("missing host ID")
	}
	host, err := host.FindOneId(h.hostID)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "getting host").Error(),
		}
	}
	if host == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", h.hostID),
		}
	}
	h.host = host
	details, err := getDetails(h.host, r)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	h.details = details
	return nil
}

// Run retrieves the next task's id given the host name and host secret by retrieving the task queue
// and popping the next task off the task queue.
func (h *hostAgentNextTask) Run(ctx context.Context) gimlet.Responder {
	begin := time.Now()

	setAgentFirstContactTime(h.host)

	grip.Error(message.WrapError(h.host.SetUserDataHostProvisioned(), message.Fields{
		"message":      "failed to mark host as done provisioning with user data",
		"host_id":      h.host.Id,
		"distro":       h.host.Distro.Id,
		"provisioning": h.host.Distro.BootstrapSettings.Method,
		"operation":    "next_task",
	}))

	defer func() {
		grip.DebugWhen(time.Since(begin) > time.Second, message.Fields{
			"message": "slow next_task operation",
			"host_id": h.host.Id,
			"distro":  h.host.Distro.Id,
			"latency": time.Since(begin).Seconds(),
		})
	}()

	nextTaskResponse, err := handleReprovisioning(ctx, h.env, h.host)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if nextTaskResponse.ShouldExit {
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	if h.details == nil {
		return gimlet.NewJSONResponse(apimodels.NextTaskResponse{
			ShouldExit: true,
		})
	}

	if checkHostHealth(h.host) {
		if err := h.fixIntentHostRunningAgent(ctx, h.host, h.details.EC2InstanceID); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "could not fix intent host that is running an agent",
				"host_id":       h.host.Id,
				"host_status":   h.host.Status,
				"operation":     "next_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": h.host.AgentRevision,
			}))
			return gimlet.NewJSONResponse(nextTaskResponse)
		}

		shouldExit, err := h.prepareHostForAgentExit(ctx, agentExitParams{
			host:          h.host,
			remoteAddr:    h.remoteAddr,
			ec2InstanceID: h.details.EC2InstanceID,
		})
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "could not prepare host for agent to exit",
				"host_id":       h.host.Id,
				"operation":     "next_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": h.host.AgentRevision,
			}))
		}

		nextTaskResponse.ShouldExit = shouldExit
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	nextTaskResponse, err = handleOldAgentRevision(nextTaskResponse, h.details, h.host)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if nextTaskResponse.ShouldExit {
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		err = errors.Wrap(err, "retrieving admin settings")
		grip.Error(err)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if flags.TaskDispatchDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), "task dispatch is disabled, returning no task")
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	// if there is already a task assigned to the host send back that task
	if h.host.RunningTask != "" {
		return sendBackRunningTask(h.host, nextTaskResponse)
	}

	var nextTask *task.Task
	var shouldRunTeardown bool

	// retrieve the next task off the task queue and attempt to assign it to the host.
	// If there is already a host that has the task, it will error
	taskQueue, err := model.LoadTaskQueue(h.host.Distro.Id)
	if err != nil {
		err = errors.Wrapf(err, "locating distro queue (%s) for host '%s'", h.host.Distro.Id, h.host.Id)
		grip.Error(err)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	// if the task queue exists, try to assign a task from it:
	if taskQueue != nil {
		// assign the task to a host and retrieve the task
		nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, taskQueue, h.taskDispatcher, h.host, h.details)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	// if we didn't find a task in the "primary" queue, then we
	// try again from the alias queue. (this code runs if the
	// primary queue doesn't exist or is empty)
	if nextTask == nil && !shouldRunTeardown {
		// if we couldn't find a task in the task queue,
		// check the alias queue...
		aliasQueue, err := model.LoadDistroAliasTaskQueue(h.host.Distro.Id)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		if aliasQueue != nil {
			nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, aliasQueue, h.taskAliasDispatcher, h.host, h.details)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
		}
	}

	// if we haven't assigned a task still, then we need to return early.
	if nextTask == nil {
		// we found a task, but it's not part of the task group so we didn't assign it
		if shouldRunTeardown {
			grip.Info(message.Fields{
				"op":      "next_task",
				"message": "host task group finished, not assigning task",
				"host_id": h.host.Id,
			})
			nextTaskResponse.ShouldTeardownGroup = true
		} else {
			// if the task is empty, still send it with a status ok and check it on the other side
			grip.Info(message.Fields{
				"op":      "next_task",
				"message": "no task to assign to host",
				"host_id": h.host.Id,
			})
		}

		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	// otherwise we've dispatched a task, so we
	// mark the task as dispatched
	if err = model.MarkHostTaskDispatched(nextTask, h.host); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	setNextTask(nextTask, &nextTaskResponse)
	return gimlet.NewJSONResponse(nextTaskResponse)
}

// prepareHostForAgentExit prepares a host to stop running tasks on the host.
// For a quarantined host, it shuts down the agent and agent monitor to prevent
// it from running further tasks. This is especially important for quarantining
// hosts, as the host may continue running, but it must stop all agent-related
// activity for now. For a terminated host, the host should already have been
// terminated but is nonetheless alive, so terminate it again.
func (h *hostAgentNextTask) prepareHostForAgentExit(ctx context.Context, params agentExitParams) (shouldExit bool, err error) {
	switch params.host.Status {
	case evergreen.HostQuarantined:
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := params.host.StopAgentMonitor(ctx, h.env); err != nil {
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

// fixIntentHostRunningAgent handles an exceptional case in which an ephemeral
// host is still believed to be an intent host but somehow the agent is running
// on an EC2 instance associated with that intent host.
func (h *hostAgentNextTask) fixIntentHostRunningAgent(ctx context.Context, host *host.Host, instanceID string) error {
	if !cloud.IsEc2Provider(host.Provider) {
		// Intent host issues only affect ephemeral (i.e. EC2) hosts.
		return nil
	}
	if cloud.IsEC2InstanceID(host.Id) {
		// If the host already has an instance ID, it's not an intent host.
		return nil
	}
	if instanceID == "" {
		// If the host is an intent host but the agent does not send the EC2
		// instance ID, there's nothing that can be done to fix it here.
		grip.Warning(message.Fields{
			"message":     "intent host has started an agent, but the agent did not provide an instance ID for the real host",
			"host_id":     host.Id,
			"host_status": host.Status,
			"provider":    host.Provider,
			"distro":      host.Distro.Id,
		})
		return nil
	}

	switch host.Status {
	case evergreen.HostBuilding:
		return errors.Wrap(h.transitionIntentHostToStarting(ctx, host, instanceID), "starting intent host that actually succeeded")
	case evergreen.HostBuildingFailed, evergreen.HostTerminated:
		return errors.Wrap(h.transitionIntentHostToDecommissioned(ctx, host, instanceID), "decommissioning intent host")
	default:
		return errors.Errorf("logical error: intent host is in state '%s', which should be impossible when the agent is running", host.Status)
	}
}

// transitionIntentHostToStarting converts an intent host to a real host
// because it's alive in the cloud. It is marked as starting to indicate that
// the host has started and can run tasks.
func (h *hostAgentNextTask) transitionIntentHostToStarting(ctx context.Context, hostToStart *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - found EC2 instance running an agent, but Evergreen believes the host still an intent host",
		"host_id":     hostToStart.Id,
		"host_tag":    hostToStart.Tag,
		"distro":      hostToStart.Distro.Id,
		"instance_id": instanceID,
		"host_status": hostToStart.Status,
	})

	intentHostID := hostToStart.Id
	hostToStart.Id = instanceID
	hostToStart.Status = evergreen.HostStarting
	if err := host.UnsafeReplace(ctx, h.env, intentHostID, hostToStart); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStartSucceeded(hostToStart.Id)

	return nil
}

// transitionIntentHostToDecommissioned converts an intent host to a real
// host because it's alive in the cloud. It is marked as decommissioned to
// indicate that the host is not valid and should be terminated.
func (h *hostAgentNextTask) transitionIntentHostToDecommissioned(ctx context.Context, hostToDecommission *host.Host, instanceID string) error {
	grip.Notice(message.Fields{
		"message":     "DB-EC2 state mismatch - found EC2 instance running an agent, but Evergreen believes the host is a stale building intent host",
		"host_id":     hostToDecommission.Id,
		"instance_id": instanceID,
		"host_status": hostToDecommission.Status,
	})

	intentHostID := hostToDecommission.Id
	hostToDecommission.Id = instanceID
	oldStatus := hostToDecommission.Status
	hostToDecommission.Status = evergreen.HostDecommissioned
	if err := host.UnsafeReplace(ctx, h.env, intentHostID, hostToDecommission); err != nil {
		return errors.Wrap(err, "replacing intent host with real host")
	}

	event.LogHostStatusChanged(hostToDecommission.Id, oldStatus, hostToDecommission.Status, evergreen.User, "host started agent but intent host is already considered a failure")
	grip.Info(message.Fields{
		"message":    "intent host decommissioned",
		"host_id":    hostToDecommission.Id,
		"host_tag":   hostToDecommission.Tag,
		"distro":     hostToDecommission.Distro.Id,
		"old_status": oldStatus,
	})

	return nil
}

type agentExitParams struct {
	host          *host.Host
	remoteAddr    string
	ec2InstanceID string
}

// assignNextAvailableTask gets the next task from the queue and sets the running task field
// of currentHost. If the host has finished a task group, we return true (and no task) so
// the host teardown the group before getting a new task.
func assignNextAvailableTask(ctx context.Context, taskQueue *model.TaskQueue, dispatcher model.TaskQueueItemDispatcher,
	currentHost *host.Host, details *apimodels.GetNextTaskDetails) (*task.Task, bool, error) {
	if currentHost.RunningTask != "" {
		grip.Error(message.Fields{
			"message":      "tried to assign task to a host already running task",
			"running_task": currentHost.RunningTask,
		})
		return nil, false, errors.New("cannot assign a task to a host with a running task")
	}
	distroToMonitor := "rhel80-medium"
	runId := utility.RandomString()
	stepStart := time.Now()
	funcStart := stepStart

	var spec model.TaskSpec
	if currentHost.LastTask != "" {
		spec = model.TaskSpec{
			Group:        currentHost.LastGroup,
			BuildVariant: currentHost.LastBuildVariant,
			Project:      currentHost.LastProject,
			Version:      currentHost.LastVersion,
		}
	}

	d, err := distro.FindOneId(currentHost.Distro.Id)
	if err != nil || d == nil {
		// Should we bailout if there is a database error leaving us unsure if the distro document actually exists?
		m := "database error while retrieving distro document;"
		if d == nil {
			m = "cannot find the db.distro document for the given distro;"
		}
		grip.Warning(message.Fields{
			"message":   m + " falling back to host.Distro",
			"distro_id": currentHost.Distro.Id,
			"host_id":   currentHost.Id,
		})
		d = &currentHost.Distro
	}
	grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
		"message":     "assignNextAvailableTask performance",
		"step":        "distro.FindOne",
		"duration_ns": time.Since(stepStart),
		"run_id":      runId,
	})
	stepStart = time.Now()

	var amiUpdatedTime time.Time
	if d.GetDefaultAMI() != currentHost.GetAMI() {
		amiEvent, err := event.FindLatestAMIModifiedDistroEvent(d.Id)
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "problem getting AMI event log",
			"host_id":   currentHost.Id,
			"distro_id": d.Id,
		}))
		amiUpdatedTime = amiEvent.Timestamp
	}

	// This loop does the following:
	// 1. Find the next task in the queue.
	// 2. Assign the task to the host.
	// 3. Dequeue the task from the in-memory and DB queue.
	//
	// Note that updating the running task on the host must occur before
	// dequeueing the task. If these two steps were in the inverse order,
	// there would be a race that can cause two hosts to run the first two
	// tasks of a 1-host task group simultaneously, i.e., if one host is
	// between dequeueing and assigning the task to itself while a second
	// host gets the task queue.
	//
	// Note also that this is not a loop over the task queue items. The loop
	// continues until the task queue is empty. This means that every
	// continue must be preceded by dequeueing the current task from the
	// queue to prevent an infinite loop.
	for taskQueue.Length() != 0 {
		if err = ctx.Err(); err != nil {
			return nil, false, errors.WithStack(err)
		}

		var queueItem *model.TaskQueueItem
		switch d.DispatcherSettings.Version {
		case evergreen.DispatcherVersionRevised, evergreen.DispatcherVersionRevisedWithDependencies:
			queueItem, err = dispatcher.RefreshFindNextTask(d.Id, spec, amiUpdatedTime)
			if err != nil {
				return nil, false, errors.Wrap(err, "problem getting next task")
			}
		default:
			queueItem, _ = taskQueue.FindNextTask(spec)
		}

		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "RefreshFindNextTask",
			"duration_ns": time.Since(stepStart),
			"run_id":      runId,
		})
		stepStart = time.Now()
		if queueItem == nil {
			return nil, false, nil
		}

		nextTask, err := task.FindOneId(queueItem.Id)
		if err != nil {
			grip.DebugWhen(queueItem.Group != "", message.Fields{
				"message":            "retrieving next task",
				"task_id":            queueItem.Id,
				"task_group":         queueItem.Group,
				"task_build_variant": queueItem.BuildVariant,
				"task_version":       queueItem.Version,
			})
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "database error while retrieving the db.tasks document for the next task to be assigned to this host",
				"distro_id":    d.Id,
				"host_id":      currentHost.Id,
				"next_task_id": queueItem.Id,
				"last_task_id": currentHost.LastTask,
			}))
			return nil, false, err
		}
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "find task",
			"duration_ns": time.Since(stepStart),
			"run_id":      runId,
		})
		stepStart = time.Now()

		if nextTask == nil {
			grip.DebugWhen(queueItem.Group != "", message.Fields{
				"message":            "next task is nil",
				"task_id":            queueItem.Id,
				"task_group":         queueItem.Group,
				"task_build_variant": queueItem.BuildVariant,
				"task_version":       queueItem.Version,
			})
			// An error is not returned in this situation due to https://jira.mongodb.org/browse/EVG-6214
			return nil, false, nil
		}

		// validate that the task can be run, if not fetch the next one in the queue.
		if !nextTask.IsHostDispatchable() {
			// Dequeue the task so we don't get it on another iteration of the loop.
			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":   "nextTask.IsHostDispatchable() is false, but there was an issue dequeuing the task",
				"distro_id": d.Id,
				"task_id":   nextTask.Id,
				"host_id":   currentHost.Id,
			}))

			continue
		}

		projectRef, err := model.FindMergedProjectRef(nextTask.Project, nextTask.Version, true)
		errMsg := message.Fields{
			"task_id":            nextTask.Id,
			"message":            "could not find project ref for next task, skipping",
			"project":            nextTask.Project,
			"host_id":            currentHost.Id,
			"task_group":         nextTask.TaskGroup,
			"task_build_variant": nextTask.BuildVariant,
			"task_version":       nextTask.Version,
		}
		if err != nil {
			grip.Alert(message.WrapError(err, errMsg))
			return nil, false, errors.Wrapf(err, "could not find project ref for next task '%s'", nextTask.Id)
		}
		if projectRef == nil {
			grip.Alert(errMsg)
			return nil, false, errors.Errorf("project ref for next task '%s' doesn't exist", nextTask.Id)
		}
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "FindMergedProjectRef",
			"duration_ns": time.Since(stepStart),
			"run_id":      runId,
		})
		stepStart = time.Now()

		isDisabled := projectRef.IsDispatchingDisabled()
		// hidden projects can only run PR tasks
		if !projectRef.IsEnabled() && (queueItem.Requester != evergreen.GithubPRRequester || !projectRef.IsHidden()) {
			isDisabled = true
		}

		if isDisabled {
			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":              "project has dispatching disabled, but there was an issue dequeuing the task",
				"distro_id":            nextTask.DistroId,
				"task_id":              nextTask.Id,
				"host_id":              currentHost.Id,
				"project":              projectRef.Id,
				"project_identifier":   projectRef.Identifier,
				"enabled":              projectRef.Enabled,
				"dispatching_disabled": projectRef.DispatchingDisabled,
			}))
			continue
		}

		// If the top task on the queue is blocked, the scheduler task queue may be out of date.
		if nextTask.Blocked() {
			grip.Debug(message.Fields{
				"message":            "top task queue task is blocked, dequeuing task",
				"host_id":            currentHost.Id,
				"distro_id":          nextTask.DistroId,
				"task_id":            nextTask.Id,
				"task_group":         nextTask.TaskGroup,
				"project":            projectRef.Id,
				"project_identifier": projectRef.Enabled,
			})
			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":            "top task queue task is blocked, but there was an issue dequeuing the task",
				"host_id":            currentHost.Id,
				"distro_id":          nextTask.DistroId,
				"task_id":            nextTask.Id,
				"task_group":         nextTask.TaskGroup,
				"project":            projectRef.Id,
				"project_identifier": projectRef.Enabled,
			}))
			continue
		}

		// If the current task group is finished we leave the task on the queue, and indicate the current group needs to be torn down.
		if details.TaskGroup != "" && details.TaskGroup != nextTask.TaskGroup {
			grip.DebugWhen(nextTask.TaskGroup != "", message.Fields{
				"message":              "not updating running task group task, because current group needs to be torn down",
				"task_distro_id":       nextTask.DistroId,
				"task_id":              nextTask.Id,
				"task_group":           nextTask.TaskGroup,
				"task_build_variant":   nextTask.BuildVariant,
				"task_version":         nextTask.Version,
				"task_project":         nextTask.Project,
				"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
			})
			return nil, true, nil
		}

		// UpdateRunningTask updates the running task in the host document
		ok, err := currentHost.UpdateRunningTask(nextTask)
		if err != nil {
			return nil, false, errors.WithStack(err)
		}
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "UpdateRunningTask",
			"duration_ns": time.Since(stepStart),
			"run_id":      runId,
		})
		stepStart = time.Now()

		// It's possible for dispatchers on different app servers to race, assigning
		// different tasks in a task group to more hosts than the task group's max hosts. We
		// must therefore check that the number of hosts running this task group does not
		// exceed the max after updating the running task on the host. If it does, we back
		// out.
		//
		// If the host just ran a task in the group, then it's eligible for running
		// more tasks in the group, regardless of how many hosts are running. We only check
		// the number of hosts running this task group if the task group is new to the host.
		grip.DebugWhen(nextTask.TaskGroup != "", message.Fields{
			"message":                 "task group lock debugging",
			"task_distro_id":          nextTask.DistroId,
			"task_id":                 nextTask.Id,
			"host_id":                 currentHost.Id,
			"host_last_group":         currentHost.LastGroup,
			"host_last_build_variant": currentHost.LastBuildVariant,
			"host_last_task":          currentHost.LastTask,
			"host_last_version":       currentHost.LastVersion,
			"host_last_project":       currentHost.LastProject,
			"task_group":              nextTask.TaskGroup,
			"task_build_variant":      nextTask.BuildVariant,
			"task_version":            nextTask.Version,
			"task_project":            nextTask.Project,
			"task_group_max_hosts":    nextTask.TaskGroupMaxHosts,
			"task_group_order":        nextTask.TaskGroupOrder,
		})

		if ok && isTaskGroupNewToHost(currentHost, nextTask) {
			dispatchRace := ""
			minTaskGroupOrderNum := 0
			if nextTask.TaskGroupMaxHosts == 1 {
				// regardless of how many hosts are running tasks, if this host is running the earliest task in the task group we should continue
				minTaskGroupOrderNum, err = host.MinTaskGroupOrderRunningByTaskSpec(nextTask.TaskGroup, nextTask.BuildVariant, nextTask.Project, nextTask.Version)
				if err != nil {
					return nil, false, errors.WithStack(err)
				}
				// if minTaskGroupOrderNum is 0 then some host doesn't have order cached, revert to previous logic
				if minTaskGroupOrderNum != 0 && minTaskGroupOrderNum < nextTask.TaskGroupOrder {
					dispatchRace = fmt.Sprintf("current task is order %d but another host is running %d", nextTask.TaskGroupOrder, minTaskGroupOrderNum)
				} else if nextTask.TaskGroupOrder > 1 {
					// If the previous task in the group has yet to run and should run, then wait for it.
					tgTasks, err := task.FindTaskGroupFromBuild(nextTask.BuildId, nextTask.TaskGroup)
					if err != nil {
						return nil, false, errors.WithStack(err)
					}
					for _, tgTask := range tgTasks {
						if tgTask.TaskGroupOrder == nextTask.TaskGroupOrder {
							break
						}
						if tgTask.TaskGroupOrder < nextTask.TaskGroupOrder && tgTask.IsHostDispatchable() && !tgTask.Blocked() {
							dispatchRace = fmt.Sprintf("an earlier task ('%s') in the task group is still dispatchable", tgTask.DisplayName)
						}
					}
				}
			}
			grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
				"message":     "assignNextAvailableTask performance",
				"step":        "find task group",
				"duration_ns": time.Since(stepStart),
				"run_id":      runId,
			})
			stepStart = time.Now()
			// for multiple-host task groups and single-host task groups without order cached
			if minTaskGroupOrderNum == 0 && dispatchRace == "" {
				numHosts, err := host.NumHostsByTaskSpec(nextTask.TaskGroup, nextTask.BuildVariant, nextTask.Project, nextTask.Version)
				if err != nil {
					return nil, false, errors.WithStack(err)
				}
				if numHosts > nextTask.TaskGroupMaxHosts {
					dispatchRace = fmt.Sprintf("tasks found on %d hosts", numHosts)
				}
			}
			grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
				"message":     "assignNextAvailableTask performance",
				"step":        "get host number",
				"duration_ns": time.Since(stepStart),
				"run_id":      runId,
			})
			stepStart = time.Now()

			if dispatchRace != "" {
				grip.Debug(message.Fields{
					"message":              "task group race, not dispatching",
					"dispatch_race":        dispatchRace,
					"task_distro_id":       nextTask.DistroId,
					"task_id":              nextTask.Id,
					"host_id":              currentHost.Id,
					"task_group":           nextTask.TaskGroup,
					"task_build_variant":   nextTask.BuildVariant,
					"task_version":         nextTask.Version,
					"task_project":         nextTask.Project,
					"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
					"task_group_order":     nextTask.TaskGroupOrder,
				})
				grip.Error(message.WrapError(currentHost.ClearRunningTask(), message.Fields{
					"message":              "problem clearing task group task from host after dispatch race",
					"dispatch_race":        dispatchRace,
					"task_distro_id":       nextTask.DistroId,
					"task_id":              nextTask.Id,
					"host_id":              currentHost.Id,
					"task_group":           nextTask.TaskGroup,
					"task_build_variant":   nextTask.BuildVariant,
					"task_version":         nextTask.Version,
					"task_project":         nextTask.Project,
					"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
				}))
				ok = false // continue loop after dequeuing task
			}
			grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
				"message":     "assignNextAvailableTask performance",
				"step":        "ClearRunningTask",
				"duration_ns": time.Since(stepStart),
				"run_id":      runId,
			})
			stepStart = time.Now()
		}

		// Dequeue the task so we don't get it on another iteration of the loop.
		grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
			"message":   "updated the relevant running task fields for the given host, but there was an issue dequeuing the task",
			"distro_id": nextTask.DistroId,
			"task_id":   nextTask.Id,
			"host_id":   currentHost.Id,
		}))
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "DequeueTask",
			"duration_ns": time.Since(stepStart),
			"run_id":      runId,
		})
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "total",
			"duration_ns": time.Since(funcStart),
			"run_id":      runId,
		})

		if !ok {
			continue
		}

		return nextTask, false, nil
	}
	return nil, false, nil
}

func isTaskGroupNewToHost(h *host.Host, t *task.Task) bool {
	return t.TaskGroup != "" &&
		(h.LastGroup != t.TaskGroup ||
			h.LastBuildVariant != t.BuildVariant ||
			h.LastProject != t.Project ||
			h.LastVersion != t.Version)
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

func handleReprovisioning(ctx context.Context, env evergreen.Environment, h *host.Host) (apimodels.NextTaskResponse, error) {
	var response apimodels.NextTaskResponse
	if h.NeedsReprovision == host.ReprovisionNone {
		return apimodels.NextTaskResponse{}, nil
	}
	if !utility.StringSliceContains([]string{evergreen.HostProvisioning, evergreen.HostRunning}, h.Status) {
		return apimodels.NextTaskResponse{}, nil
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()
	if err := h.StopAgentMonitor(stopCtx, env); err != nil {
		// Stopping the agent monitor should not stop reprovisioning as long as
		// the host is not currently running a task.
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "problem stopping agent monitor for reprovisioning",
			"host_id":       h.Id,
			"operation":     "next_task",
			"revision":      evergreen.BuildRevision,
			"agent":         evergreen.AgentVersion,
			"current_agent": h.AgentRevision,
		}))
	}

	if err := prepareForReprovision(ctx, env, h); err != nil {
		return apimodels.NextTaskResponse{}, err
	}
	response.ShouldExit = true
	return response, nil
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

func getDetails(h *host.Host, r *http.Request) (*apimodels.GetNextTaskDetails, error) {
	isOldAgent := agentRevisionIsOld(h)
	// if agent revision is old, we should indicate an exit if there are errors
	details := &apimodels.GetNextTaskDetails{}
	if err := utility.ReadJSON(r.Body, details); err != nil {
		if isOldAgent {
			if innerErr := h.SetNeedsNewAgent(true); innerErr != nil {
				grip.Error(message.WrapError(innerErr, message.Fields{
					"host_id":       h.Id,
					"operation":     "next_task",
					"message":       "problem indicating that host needs new agent",
					"source":        "database error",
					"revision":      evergreen.BuildRevision,
					"agent":         evergreen.AgentVersion,
					"current_agent": h.AgentRevision,
				}))
				return nil, innerErr
			}
		}
		grip.Info(message.WrapError(err, message.Fields{
			"host_id":       h.Id,
			"operation":     "next_task",
			"message":       "unable to unmarshal next task details",
			"host_revision": h.AgentRevision,
			"revision":      evergreen.BuildRevision,
			"agent":         evergreen.AgentVersion,
		}))
		if isOldAgent {
			return nil, nil
		}
	}
	return details, nil
}

// prepareForReprovision readies a host for reprovisioning.
func prepareForReprovision(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	if err := h.MarkAsReprovisioning(); err != nil {
		return errors.Wrap(err, "marking host as ready for reprovisioning")
	}

	// Enqueue the job immediately, if possible.
	if err := units.EnqueueHostReprovisioningJob(ctx, env, h); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":           "could not enqueue job to reprovision host",
			"host_id":           h.Id,
			"needs_reprovision": h.NeedsReprovision,
		}))
	}

	return nil
}

func setAgentFirstContactTime(h *host.Host) {
	if !h.AgentStartTime.IsZero() {
		return
	}

	if err := h.SetAgentStartTime(); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not set host's agent start time for first contact",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
		return
	}

	grip.InfoWhen(h.Provider != evergreen.ProviderNameStatic, message.Fields{
		"message":                   "agent initiated first contact with server",
		"host_id":                   h.Id,
		"distro":                    h.Distro.Id,
		"provisioning":              h.Distro.BootstrapSettings.Method,
		"agent_start_duration_secs": time.Since(h.CreationTime).Seconds(),
	})
}

func handleOldAgentRevision(response apimodels.NextTaskResponse, details *apimodels.GetNextTaskDetails, h *host.Host) (apimodels.NextTaskResponse, error) {
	if !agentRevisionIsOld(h) {
		return response, nil
	}

	// Non-legacy hosts deploying agents via the agent monitor may be
	// running an agent on the current revision, but the database host has
	// yet to be updated.
	if !h.Distro.LegacyBootstrap() && details.AgentRevision != h.AgentRevision {
		err := h.SetAgentRevision(details.AgentRevision)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":        "problem updating host agent revision",
				"operation":      "NextTask",
				"host_id":        h.Id,
				"host_tag":       h.Tag,
				"source":         "database error",
				"host_revision":  details.AgentRevision,
				"agent_version":  evergreen.AgentVersion,
				"build_revision": evergreen.BuildRevision,
			}))
		}

		event.LogHostAgentDeployed(h.Id)
		grip.Info(message.Fields{
			"message":        "updated host agent revision",
			"operation":      "NextTask",
			"host_id":        h.Id,
			"host_tag":       h.Tag,
			"source":         "database error",
			"host_revision":  details.AgentRevision,
			"agent_version":  evergreen.AgentVersion,
			"build_revision": evergreen.BuildRevision,
		})
		return response, nil
	}

	if details.TaskGroup == "" {
		if err := h.SetNeedsNewAgent(true); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":        h.Id,
				"operation":      "NextTask",
				"message":        "problem indicating that host needs new agent",
				"source":         "database error",
				"build_revision": evergreen.BuildRevision,
				"agent_version":  evergreen.AgentVersion,
				"host_revision":  h.AgentRevision,
			}))
			return apimodels.NextTaskResponse{}, err

		}
		if err := h.ClearRunningTask(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":        h.Id,
				"operation":      "next_task",
				"message":        "problem unsetting running task",
				"source":         "database error",
				"build_revision": evergreen.BuildRevision,
				"agent_version":  evergreen.AgentVersion,
				"host_revision":  h.AgentRevision,
			}))
			return apimodels.NextTaskResponse{}, err
		}
		response.ShouldExit = true
		return response, nil
	}

	return response, nil
}

func sendBackRunningTask(h *host.Host, response apimodels.NextTaskResponse) gimlet.Responder {
	var err error
	var t *task.Task
	t, err = task.FindOneId(h.RunningTask)
	if err != nil {
		err = errors.Wrapf(err, "getting running task %s", h.RunningTask)
		grip.Error(err)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	// if the task can be dispatched and activated dispatch it
	if t.IsHostDispatchable() {
		err = errors.WithStack(model.MarkHostTaskDispatched(t, h))
		if err != nil {
			grip.Error(errors.Wrapf(err, "while marking task %s as dispatched for host %s", t.Id, h.Id))
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}
	// if the task is activated return that task
	if t.Activated {
		setNextTask(t, &response)
		return gimlet.NewJSONResponse(response)
	}
	// the task is not activated so the host's running task should be unset
	// so it can retrieve a new task.
	if err = h.ClearRunningTask(); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	// return an empty
	grip.Info(message.Fields{
		"op":      "next_task",
		"message": "unset running task field for inactive task on host",
		"host_id": h.Id,
		"task_id": t.Id,
	})
	return gimlet.NewJSONResponse(response)
}

// setNextTask constructs a NextTaskResponse from a task that has been assigned to run next.
func setNextTask(t *task.Task, response *apimodels.NextTaskResponse) {
	response.TaskId = t.Id
	response.TaskSecret = t.Secret
	response.TaskGroup = t.TaskGroup
	response.Version = t.Version
	response.Build = t.BuildId
}

// POST /rest/v2/hosts/{host_id}/task/{task_id}/end

type hostAgentEndTask struct {
	env        evergreen.Environment
	hostID     string
	taskID     string
	remoteAddr string
	details    apimodels.TaskEndDetail
}

func makeHostAgentEndTask(env evergreen.Environment) gimlet.RouteHandler {
	return &hostAgentEndTask{
		env: env,
	}
}

func (h *hostAgentEndTask) Factory() gimlet.RouteHandler {
	return &hostAgentEndTask{
		env: h.env,
	}
}

func (h *hostAgentEndTask) Parse(ctx context.Context, r *http.Request) error {
	h.remoteAddr = r.RemoteAddr
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if h.hostID = gimlet.GetVars(r)["host_id"]; h.hostID == "" {
		return errors.New("missing host ID")
	}
	if err := utility.ReadJSON(r.Body, &h.details); err != nil {
		message := fmt.Sprintf("reading task end details for task  %s: %v", h.taskID, err)
		grip.Error(message)
		return errors.Wrap(err, message)
	}
	return nil
}

// Run creates test results from the request and the project config.
// It then acquires the lock, and with it, marks tasks as finished or inactive if aborted.
// If the task is a patch, it will alert the users based on failures
// It also updates the expected task duration of the task for scheduling.
func (h *hostAgentEndTask) Run(ctx context.Context) gimlet.Responder {
	finishTime := time.Now()

	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	currentHost, err := host.FindOneId(h.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting host"))
	}
	if currentHost == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", h.hostID)},
		)
	}

	endTaskResp := &apimodels.EndTaskResponse{}
	// Check that status is either finished or aborted (i.e. undispatched)
	if !evergreen.IsValidTaskEndStatus(h.details.Status) && h.details.Status != evergreen.TaskUndispatched {
		msg := fmt.Errorf("invalid end status '%s' for task %s", h.details.Status, t.Id)
		return gimlet.MakeJSONErrorResponder(msg)
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
			"task_details_from_agent": h.details,
			"host_id":                 currentHost.Id,
			"distro":                  currentHost.Distro.Id,
		})
		endTaskResp.ShouldExit = true
		return gimlet.NewJSONResponse(endTaskResp)
	}

	// For a single-host task group, if a task fails, block and dequeue later tasks in that group.
	// Call before MarkEnd so the version is marked finished when this is the last task in the version to finish,
	// and before clearing the running task from the host so later tasks in the group aren't picked up by the host.
	model.CheckAndBlockSingleHostTaskGroup(t, h.details.Status)

	projectRef, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "empty projectRef for task",
		})
	}

	// mark task as finished
	deactivatePrevious := utility.FromBoolPtr(projectRef.DeactivatePrevious)
	err = model.MarkEnd(t, evergreen.APIServerTaskActivator, finishTime, &h.details, deactivatePrevious)
	if err != nil {
		err = errors.Wrapf(err, "calling mark finish on task %s", t.Id)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	// Clear the running task on the host now that the task has finished.
	if err = currentHost.ClearRunningAndSetLastTask(t); err != nil {
		err = errors.Wrapf(err, "clearing running task %s for host %s", t.Id, currentHost.Id)
		grip.Errorf(err.Error())
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	if t.Requester == evergreen.MergeTestRequester {
		if err = model.HandleEndTaskForCommitQueueTask(t, h.details.Status); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// the task was aborted if it is still in undispatched.
	// the active state should be inactive.
	if h.details.Status == evergreen.TaskUndispatched {
		if t.Activated {
			grip.Warningf("task %s is active and undispatched after being marked as finished", t.Id)
			return gimlet.NewJSONResponse(struct{}{})
		}
		abortMsg := fmt.Sprintf("task %s has been aborted and will not run", t.Id)
		grip.Infof(abortMsg)
		return gimlet.NewJSONResponse(&apimodels.EndTaskResponse{})
	}

	queue := h.env.RemoteQueue()
	job := units.NewCollectTaskEndDataJob(*t, currentHost, nil, currentHost.Id)
	if err = queue.Put(ctx, job); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "couldn't queue job to update task stats accounting"))
	}

	if checkHostHealth(currentHost) {
		if _, err := prepareHostForAgentExit(ctx, agentExitParams{
			host:       currentHost,
			remoteAddr: h.remoteAddr,
		}, h.env); err != nil {
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
			grip.Error(message.WrapError(units.HandlePoisonedHost(ctx, h.env, currentHost, msg), message.Fields{
				"message": "unable to disable poisoned host",
				"host":    currentHost.Id,
			}))
		}

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, h.env); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor",
				"host_id":       currentHost.Id,
				"operation":     "end_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": currentHost.AgentRevision,
			}))
			return gimlet.MakeJSONInternalErrorResponder(err)
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
		"status":      h.details.Status,
		"path":        fmt.Sprintf("/rest/v2/hosts/%s/task/%s/end", currentHost.Id, t.Id),
	}

	if t.IsPartOfDisplay() {
		msg["display_task_id"] = t.DisplayTaskId
	}

	grip.Info(msg)
	return gimlet.NewJSONResponse(endTaskResp)
}

// prepareHostForAgentExit prepares a host to stop running tasks on the host.
// For a quarantined host, it shuts down the agent and agent monitor to prevent
// it from running further tasks. This is especially important for quarantining
// hosts, as the host may continue running, but it must stop all agent-related
// activity for now. For a terminated host, the host should already have been
// terminated but is nonetheless alive, so terminate it again.
func prepareHostForAgentExit(ctx context.Context, params agentExitParams, env evergreen.Environment) (shouldExit bool, err error) {
	switch params.host.Status {
	case evergreen.HostQuarantined:
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := params.host.StopAgentMonitor(ctx, env); err != nil {
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
