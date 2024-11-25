package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
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
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// if a host encounters more than this number of system failures, then it should be disabled.
const consecutiveSystemFailureThreshold = 3

// if we fail to clean up the agent on a quarantined host more than this number of times, we can consider the
// host unreachable and clear its secret.
const hostAgentCleanupLimit = 10

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
	host, err := host.FindOneId(ctx, h.hostID)
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

	setAgentFirstContactTime(ctx, h.host)

	if err := h.host.UnsetTaskGroupTeardownStartTime(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	grip.Error(message.WrapError(h.host.SetUserDataHostProvisioned(ctx), message.Fields{
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
		shouldExit, err := prepareHostForAgentExit(ctx, agentExitParams{
			host:       h.host,
			remoteAddr: h.remoteAddr,
		}, h.env)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "could not prepare host for agent to exit",
				"host_id":       h.host.Id,
				"operation":     "next_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": h.host.AgentRevision,
			}))
			if err = h.host.IncrementNumAgentCleanupFailures(ctx); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(err)
			}
			if h.host.NumAgentCleanupFailures > hostAgentCleanupLimit {
				if err = h.host.CreateSecret(ctx, true); err != nil {
					return gimlet.MakeJSONInternalErrorResponder(err)
				}
			}
		}

		nextTaskResponse.ShouldExit = shouldExit
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	nextTaskResponse, err = handleOldAgentRevision(ctx, nextTaskResponse, h.details, h.host)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if nextTaskResponse.ShouldExit {
		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	flags, err := evergreen.GetServiceFlags(ctx)
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
		return sendBackRunningTask(ctx, h.env, h.host, nextTaskResponse)
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
		nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, h.env, taskQueue, h.taskDispatcher, h.host, h.details)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// if we didn't find a task in the "primary" queue, then we
	// try again from the alias queue. (this code runs if the
	// primary queue doesn't exist or is empty)
	if nextTask == nil && !shouldRunTeardown {
		// if we couldn't find a task in the task queue,
		// check the alias queue...
		secondaryQueue, err := model.LoadDistroSecondaryTaskQueue(h.host.Distro.Id)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		if secondaryQueue != nil {
			nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, h.env, secondaryQueue, h.taskAliasDispatcher, h.host, h.details)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(err)
			}
		}
	}

	// if we haven't assigned a task still, then we need to return early.
	if nextTask == nil {
		// we found a task, but it's not part of the task group so we didn't assign it
		if shouldRunTeardown {
			grip.Info(message.Fields{
				"op":         "next_task",
				"message":    "host task group finished, not assigning task and instead requesting host to run teardown group",
				"host_id":    h.host.Id,
				"task_group": h.details.TaskGroup,
			})
			err = h.host.SetTaskGroupTeardownStartTime(ctx)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(err)
			}
			nextTaskResponse.ShouldTeardownGroup = true
		} else {
			// if the task is empty, still send it with a status ok and check it on the other side
			grip.Info(message.Fields{
				"op":      "next_task",
				"message": "no task to assign to host",
				"host_id": h.host.Id,
			})
		}

		nextTaskResponse.EstimatedMaxIdleDuration = h.host.Distro.HostAllocatorSettings.AcceptableHostIdleTime
		if nextTaskResponse.EstimatedMaxIdleDuration == 0 {
			schedulerConfig := evergreen.SchedulerConfig{}
			err := schedulerConfig.Get(ctx)

			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem getting scheduler config for idle threshold to send for next task",
			}))
			if err == nil {
				nextTaskResponse.EstimatedMaxIdleDuration = time.Duration(schedulerConfig.AcceptableHostIdleTimeSeconds) * time.Second
			}
		}

		return gimlet.NewJSONResponse(nextTaskResponse)
	}

	setNextTask(nextTask, &nextTaskResponse)
	return gimlet.NewJSONResponse(nextTaskResponse)
}

// assignNextAvailableTask gets the next task from the queue and sets the running task field
// of currentHost. If the host has finished a task group, we return true (and no task) so
// the host teardown the group before getting a new task.
func assignNextAvailableTask(ctx context.Context, env evergreen.Environment, taskQueue *model.TaskQueue, dispatcher model.TaskQueueItemDispatcher,
	currentHost *host.Host, details *apimodels.GetNextTaskDetails) (*task.Task, bool, error) {
	if currentHost.RunningTask != "" {
		grip.Error(message.Fields{
			"message":      "tried to assign task to a host already running task",
			"running_task": currentHost.RunningTask,
			"execution":    currentHost.RunningTaskExecution,
		})
		return nil, false, errors.New("cannot assign a task to a host with a running task")
	}

	var spec model.TaskSpec
	if currentHost.LastTask != "" {
		spec = model.TaskSpec{
			Group:        currentHost.LastGroup,
			BuildVariant: currentHost.LastBuildVariant,
			Project:      currentHost.LastProject,
			Version:      currentHost.LastVersion,
		}
	}

	d, err := distro.FindOneId(ctx, currentHost.Distro.Id)
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
	// Note that assigning the task to the host must occur before
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
		case evergreen.DispatcherVersionRevisedWithDependencies:
			queueItem, err = dispatcher.RefreshFindNextTask(ctx, d.Id, spec, amiUpdatedTime)
			if err != nil {
				return nil, false, errors.Wrap(err, "problem getting next task")
			}
		default:
			return nil, false, errors.Errorf("invalid dispatcher version '%s' for host '%s'", d.DispatcherSettings.Version, currentHost.Id)
		}

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

		if nextTask == nil {
			grip.DebugWhen(queueItem.Group != "", message.Fields{
				"message":            "next task is nil",
				"task_id":            queueItem.Id,
				"task_group":         queueItem.Group,
				"task_build_variant": queueItem.BuildVariant,
				"task_version":       queueItem.Version,
			})
			// An error is not returned in this situation due to EVG-6214
			return nil, false, nil
		}

		// validate that the task can be run, if not fetch the next one in the queue.
		if !nextTask.IsHostDispatchable() {
			grip.Warning(message.Fields{
				"message": "task was not dispatchable",
				"task":    nextTask.Id,
				"variant": nextTask.BuildVariant,
				"project": nextTask.Project,
			})
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
			"distro_id":          d.Id,
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

		isDisabled := projectRef.IsDispatchingDisabled()
		// hidden projects can only run PR tasks
		if !projectRef.Enabled && (queueItem.Requester != evergreen.GithubPRRequester || !projectRef.IsHidden()) {
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
			grip.Debug(message.Fields{
				"message":              "next task is a standalone task or part of a different task group; not updating running task group task, because current task group needs to be torn down",
				"current_task_group":   details.TaskGroup,
				"task_distro_id":       nextTask.DistroId,
				"task_id":              nextTask.Id,
				"next_task_group":      nextTask.TaskGroup,
				"task_build_variant":   nextTask.BuildVariant,
				"task_version":         nextTask.Version,
				"task_project":         nextTask.Project,
				"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
			})
			return nil, true, nil
		}

		lockErr := dispatchHostTaskAtomically(ctx, env, currentHost, nextTask)
		if err != nil && !db.IsDuplicateKey(lockErr) {
			return nil, false, errors.Wrapf(err, "dispatching task '%s' to host '%s'", nextTask.Id, currentHost.Id)
		}
		dispatchedTask := lockErr == nil

		if dispatchedTask && isTaskGroupNewToHost(currentHost, nextTask) {
			// If the host just ran a task in the group, then it's eligible for
			// running more tasks in the group, regardless of how many other
			// hosts are running tasks in the task group. Only check the number
			// of hosts running this task group if the task group is new to the
			// host.
			if err := checkHostTaskGroupAfterDispatch(ctx, nextTask); err != nil {
				grip.Debug(message.Fields{
					"message":              "failed dispatch task group check due to race, not dispatching",
					"task_distro_id":       nextTask.DistroId,
					"task_id":              nextTask.Id,
					"host_id":              currentHost.Id,
					"dispatch_race":        err.Error(),
					"task_group":           nextTask.TaskGroup,
					"task_build_variant":   nextTask.BuildVariant,
					"task_version":         nextTask.Version,
					"task_project":         nextTask.Project,
					"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
					"task_group_order":     nextTask.TaskGroupOrder,
				})
				if err := undoHostTaskDispatchAtomically(ctx, env, currentHost, nextTask); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":              "problem undoing task group task dispatch after dispatch race",
						"dispatch_race":        err.Error(),
						"task_distro_id":       nextTask.DistroId,
						"task_id":              nextTask.Id,
						"host_id":              currentHost.Id,
						"task_group":           nextTask.TaskGroup,
						"task_build_variant":   nextTask.BuildVariant,
						"task_version":         nextTask.Version,
						"task_project":         nextTask.Project,
						"task_group_max_hosts": nextTask.TaskGroupMaxHosts,
					}))
				}

				// Continue on trying to dispatch a different task.
				dispatchedTask = false
			}
		}

		// Dequeue the task so we don't get it on another iteration of the loop.
		grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
			"message":   "updated the relevant running task fields for the given host, but there was an issue dequeuing the task",
			"distro_id": nextTask.DistroId,
			"task_id":   nextTask.Id,
			"host_id":   currentHost.Id,
		}))

		if !dispatchedTask {
			grip.Warning(message.Fields{
				"message": "task was not dispatched",
				"task":    nextTask.Id,
				"variant": nextTask.BuildVariant,
				"project": nextTask.Project,
			})
			continue
		}

		grip.Error(message.WrapError(nextTask.IncNumNextTaskDispatches(), message.Fields{
			"message":        "problem updating the number of times the task has been dispatched",
			"task_id":        nextTask.Id,
			"task_execution": nextTask.Execution,
			"host_id":        currentHost.Id,
			"distro_id":      d.Id,
		}))

		return nextTask, false, nil
	}

	if taskQueue.Length() == 0 && details.TaskGroup != "" {
		grip.Debug(message.Fields{
			"message":           "task queue is empty while task group is running, meaning there are no task group tasks remaining and the host should run teardown group",
			"task_group":        details.TaskGroup,
			"host_id":           currentHost.Id,
			"distro_id":         d.Id,
			"task_queue_is_nil": taskQueue == nil,
			"task_queue":        fmt.Sprintf("%#v", taskQueue),
		})
		// If we have reached the end of the queue and the previous task was part of a task group,
		// the current task group is finished and needs to be torn down.
		return nil, true, nil
	}

	return nil, false, nil
}

// checkHostTaskGroupAfterDispatch checks that the task group max hosts is
// still respected after the task has already been assigned to the host and that
// for single-host task groups, it is dispatching the next task in the group. It
// will return an error if the task is not dispatchable; otherwise, it returns
// true.
//
// The reason this check is necessary is that it's possible for dispatchers on
// different app servers to race, assigning different tasks in a task group to
// more hosts than the task group's max hosts. Therefore, we must check that the
// number of hosts running this task group does not exceed the max after
// assigning the task to the host. If it exceeds the max host limit, this will
// return true to indicate that the host should not run this task.
func checkHostTaskGroupAfterDispatch(ctx context.Context, t *task.Task) error {
	var minTaskGroupOrderNum int
	if t.IsPartOfSingleHostTaskGroup() {
		var err error
		// Regardless of how many hosts are running tasks, if this host is
		// running the earliest task in the task group we should continue.
		minTaskGroupOrderNum, err = host.MinTaskGroupOrderRunningByTaskSpec(ctx, t.TaskGroup, t.BuildVariant, t.Project, t.Version)
		if err != nil {
			return errors.Wrap(err, "getting min task group order")
		}
		// minTaskGroupOrderNum is only available if a host has that information
		// cached, which was not always the case. If some host doesn't have the
		// task group order cached, then minTaskGroupOrderNum will be 0. For
		// backward compatibility in case it is 0, this will later fall back to
		// counting the hosts to check that max hosts is respected.
		if minTaskGroupOrderNum != 0 && minTaskGroupOrderNum < t.TaskGroupOrder {
			return errors.Errorf("current task is order %d but another host is running %d", t.TaskGroupOrder, minTaskGroupOrderNum)
		}
		if t.TaskGroupOrder > 1 {
			// If the previous task in the single-host task group has yet to run
			// and should run, then wait for the previous task to run.
			tgTasks, err := task.FindTaskGroupFromBuild(t.BuildId, t.TaskGroup)
			if err != nil {
				return errors.Wrap(err, "finding task group from build")
			}
			for _, tgTask := range tgTasks {
				if tgTask.TaskGroupOrder == t.TaskGroupOrder {
					break
				}
				if tgTask.TaskGroupOrder < t.TaskGroupOrder && tgTask.IsHostDispatchable() && !tgTask.Blocked() {
					return errors.Errorf("an earlier task in the task group ('%s') is still dispatchable", tgTask.DisplayName)
				}
			}
		}
	}

	// For multiple-host task groups and single-host task groups without order
	// cached in the host, check that max hosts is respected.
	if minTaskGroupOrderNum == 0 {
		numHosts, err := host.NumHostsByTaskSpec(ctx, t.TaskGroup, t.BuildVariant, t.Project, t.Version)
		if err != nil {
			return errors.Wrap(err, "getting number of hosts running task group")
		}
		if numHosts > t.TaskGroupMaxHosts {
			return errors.Errorf("tasks found on %d hosts, which exceeds max hosts %d", numHosts, t.TaskGroupMaxHosts)
		}
	}

	return nil
}

func dispatchHostTaskAtomically(ctx context.Context, env evergreen.Environment, h *host.Host, t *task.Task) error {
	if err := func() error {
		session, err := env.Client().StartSession()
		if err != nil {
			return errors.Wrap(err, "starting transaction session")
		}
		defer session.EndSession(ctx)

		if _, err := session.WithTransaction(ctx, dispatchHostTask(env, h, t)); err != nil {
			return err
		}

		return nil
	}(); err != nil {
		return err
	}

	event.LogHostTaskDispatched(t.Id, t.Execution, h.Id)
	event.LogHostRunningTaskSet(h.Id, t.Id, t.Execution)

	if t.IsPartOfDisplay() {
		// The task is already dispatched at this point, so continue if this
		// errors.
		grip.Error(message.WrapError(model.UpdateDisplayTaskForTask(t), message.Fields{
			"message":      "could not update parent display task after dispatching task",
			"task":         t.Id,
			"display_task": t.DisplayTaskId,
			"host":         h.Id,
		}))
	}

	return nil
}

func dispatchHostTask(env evergreen.Environment, h *host.Host, t *task.Task) func(mongo.SessionContext) (interface{}, error) {
	return func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := h.UpdateRunningTaskWithContext(sessCtx, env, t); err != nil {
			return nil, errors.Wrapf(err, "updating running task for host '%s' to '%s'", h.Id, t.Id)
		}

		dispatchedAt := time.Now()
		if err := t.MarkAsHostDispatchedWithContext(sessCtx, env, h.Id, h.Distro.Id, h.AgentRevision, dispatchedAt); err != nil {
			return nil, errors.Wrapf(err, "marking task '%s' as dispatched to host '%s'", t.Id, h.Id)
		}

		return nil, nil
	}
}

func undoHostTaskDispatchAtomically(ctx context.Context, env evergreen.Environment, h *host.Host, t *task.Task) error {
	clearedTask := h.RunningTask
	clearedTaskExec := h.RunningTaskExecution

	if err := func() error {
		session, err := env.Client().StartSession()
		if err != nil {
			return errors.Wrap(err, "starting transaction session")
		}
		defer session.EndSession(ctx)

		if _, err := session.WithTransaction(ctx, undoHostTaskDispatch(env, h, t)); err != nil {
			return err
		}

		return nil
	}(); err != nil {
		return err
	}

	if clearedTask != "" {
		event.LogHostRunningTaskCleared(h.Id, clearedTask, clearedTaskExec)
		event.LogHostTaskUndispatched(clearedTask, clearedTaskExec, h.Id)
	}

	if t.IsPartOfDisplay() {
		// The dispatch has already been undone at this point, so continue if
		// this errors.
		grip.Error(message.WrapError(model.UpdateDisplayTaskForTask(t), message.Fields{
			"message":      "could not update parent display task after undoing task dispatch",
			"task":         t.Id,
			"display_task": t.DisplayTaskId,
			"host":         h.Id,
		}))
	}

	return nil
}

func undoHostTaskDispatch(env evergreen.Environment, h *host.Host, t *task.Task) func(mongo.SessionContext) (interface{}, error) {
	return func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := h.ClearRunningTaskWithContext(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "clearing running task '%s' execution '%d' from host '%s'", h.RunningTask, h.RunningTaskExecution, h.Id)
		}
		if err := t.MarkAsHostUndispatchedWithContext(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "marking task '%s' as no longer dispatched", t.Id)
		}
		return nil, nil
	}
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
			"host_status":   h.Status,
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
			if innerErr := h.SetNeedsNewAgent(r.Context(), true); innerErr != nil {
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
	if err := h.MarkAsReprovisioning(ctx); err != nil {
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

func setAgentFirstContactTime(ctx context.Context, h *host.Host) {
	if !h.AgentStartTime.IsZero() {
		return
	}

	if err := h.SetAgentStartTime(ctx); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not set host's agent start time for first contact",
			"host_id": h.Id,
			"distro":  h.Distro.Id,
		}))
		return
	}

	grip.InfoWhen(h.Provider != evergreen.ProviderNameStatic, message.Fields{
		"message":                             "agent initiated first contact with server",
		"host_id":                             h.Id,
		"distro":                              h.Distro.Id,
		"provisioning":                        h.Distro.BootstrapSettings.Method,
		"agent_start_duration_secs":           time.Since(h.CreationTime).Seconds(),
		"agent_start_from_billing_start_secs": time.Since(h.BillingStartTime).Seconds(),
		"agent_start_from_requested_secs":     time.Since(h.StartTime).Seconds(),
	})
}

func handleOldAgentRevision(ctx context.Context, response apimodels.NextTaskResponse, details *apimodels.GetNextTaskDetails, h *host.Host) (apimodels.NextTaskResponse, error) {
	if !agentRevisionIsOld(h) {
		return response, nil
	}

	// Non-legacy hosts deploying agents via the agent monitor may be
	// running an agent on the current revision, but the database host has
	// yet to be updated.
	if !h.Distro.LegacyBootstrap() && details.AgentRevision != h.AgentRevision {
		err := h.SetAgentRevision(ctx, details.AgentRevision)
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
		if err := h.SetNeedsNewAgent(ctx, true); err != nil {
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
		if err := h.ClearRunningTask(ctx); err != nil {
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

// sendBackRunningTask re-dispatches a task to a host that has already been
// assigned to run it.
func sendBackRunningTask(ctx context.Context, env evergreen.Environment, h *host.Host, response apimodels.NextTaskResponse) gimlet.Responder {
	getMessage := func(msg string) message.Fields {
		return message.Fields{
			"message":        msg,
			"host":           h.Id,
			"task":           h.RunningTask,
			"task_execution": h.RunningTaskExecution,
		}
	}

	grip.Info(getMessage("attempting to re-send running task back to host after it's already been assigned"))

	var err error
	var t *task.Task
	t, err = task.FindOneIdAndExecution(h.RunningTask, h.RunningTaskExecution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting running task '%s' execution '%d'", h.RunningTask, h.RunningTaskExecution))
	}
	if t == nil {
		grip.Notice(getMessage("clearing host's running task because it does not exist"))
		// Need to store the running task and execution here because
		// ClearRunningTask will unset them.
		runningTask := h.RunningTask
		runningTaskExec := h.RunningTaskExecution
		if err := h.ClearRunningTask(ctx); err != nil {
			grip.Error(message.WrapError(err, getMessage("could not clear host's nonexistent running task")))
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
		err := errors.Errorf("host's running task '%s' execution '%d' not found", runningTask, runningTaskExec)
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	if t.IsStuckTask() {
		if err := model.ClearAndResetStrandedHostTask(ctx, env.Settings(), h); err != nil {
			grip.Error(message.WrapError(err, getMessage("ending and resetting system failed task")))
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
		// The agent is expected to run the task once it's been assigned it. If it's requesting the same
		// task over and over again is a sign that something is wrong.
		msg := fmt.Sprintf("The agent has re-requested the same task '%d' times.", evergreen.MaxTaskDispatchAttempts)
		msg += " It's possible that the agent is in a bad state."

		grip.Error(message.Fields{
			"message":        msg,
			"task_id":        t.Id,
			"task_execution": t.Execution,
			"host_id":        h.Id,
			"agent_version":  h.AgentRevision,
		})

		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("The agent has re-requested the task '%s' with execution %d %d times.", t.Id, t.Execution, evergreen.MaxTaskDispatchAttempts),
		})
	}

	if isTaskGroupNewToHost(h, t) {
		if err := checkHostTaskGroupAfterDispatch(ctx, t); err != nil {
			if err := undoHostTaskDispatchAtomically(ctx, env, h, t); err != nil {
				grip.Error(message.WrapError(err, getMessage("could not undo dispatch after task group check failed")))
				return gimlet.MakeJSONInternalErrorResponder(err)
			}
			grip.Error(message.WrapError(err, getMessage("task group check had dispatch race")))
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	if t.IsHostDispatchable() {
		grip.Notice(getMessage("marking task as dispatched because it is not currently dispatched"))
		if err := model.MarkHostTaskDispatched(t, h); err != nil {
			grip.Error(message.WrapError(err, getMessage("could not mark task as dispatched to host")))
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	if t.Activated {
		grip.Error(message.WrapError(t.IncNumNextTaskDispatches(), message.Fields{
			"message":        "problem updating the number of times the task has been dispatched",
			"task_id":        t.Id,
			"task_execution": t.Execution,
			"host_id":        h.Id,
		}))
		setNextTask(t, &response)
		return gimlet.NewJSONResponse(response)
	}

	// The task is inactive, so the host's running task should be unset so it
	// can retrieve a new task.
	if err = h.ClearRunningTask(ctx); err != nil {
		grip.Error(message.WrapError(err, getMessage("could not clear host's running task after it was found to be inactive")))
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	grip.Info(getMessage("unset host's running task because task is inactive"))

	return gimlet.NewJSONResponse(response)
}

// setNextTask constructs a NextTaskResponse from a task that has been assigned to run next.
func setNextTask(t *task.Task, response *apimodels.NextTaskResponse) {
	response.TaskId = t.Id
	response.TaskExecution = t.Execution
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
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{
		attribute.String(evergreen.HostIDOtelAttribute, h.hostID),
		attribute.String(evergreen.TaskIDOtelAttribute, h.taskID),
	})
	ctx, span := tracer.Start(ctx, "host-agent-end-task")
	defer span.End()

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
	span.SetAttributes(attribute.Int(evergreen.TaskExecutionOtelAttribute, t.Execution))
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{
		attribute.Int(evergreen.TaskExecutionOtelAttribute, t.Execution),
	})

	currentHost, err := host.FindOneId(ctx, h.hostID)
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

	projectRef, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "empty project ref for task",
		})
	}

	// The order of operations here for clearing the task from the host and
	// marking the task finished is critical and must be done in this particular
	// order.
	//
	// It's possible in some edge cases for the first operation to succeed, but
	// the second one to fail (e.g. if the app server abruptly shuts down in
	// between them). This failure state has to be detected in a separate
	// cleanup job, but that job may be cheap or expensive depending on the
	// order.
	//
	// The current order of operations is:
	// 1. Clear the host's running task.
	// 2. Mark the task finished.
	// If the second operation fails to happen, this issue is easy to detect in
	// the cleanup job, because the task will be in an incomplete state and will
	// not heartbeat.
	//
	// However, if the order of operations is instead:
	// 1. Mark the task finished.
	// 2. Clear the host's running task.
	// This is a more difficult check because it will require cross-referencing
	// the host's state against the task's state. Doing the former order of
	// operations avoids this expensive check.
	if err = currentHost.ClearRunningAndSetLastTask(ctx, t); err != nil {
		err = errors.Wrapf(err, "clearing running task '%s' for host '%s'", t.Id, currentHost.Id)
		grip.Errorf(err.Error())
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	deactivatePrevious := utility.FromBoolPtr(projectRef.DeactivatePrevious)
	details := &h.details
	if t.Aborted {
		details = &apimodels.TaskEndDetail{
			Status:      evergreen.TaskFailed,
			Description: evergreen.TaskDescriptionAborted,
		}
	}
	err = model.MarkEnd(ctx, h.env.Settings(), t, evergreen.APIServerTaskActivator, finishTime, details, deactivatePrevious)
	if err != nil {
		err = errors.Wrapf(err, "calling mark finish on task '%s'", t.Id)
		return gimlet.MakeJSONInternalErrorResponder(err)
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
			grip.Warningf("task '%s' is active and undispatched after being marked as finished", t.Id)
			return gimlet.NewJSONResponse(&apimodels.EndTaskResponse{})
		}
		abortMsg := fmt.Sprintf("task '%s' has been aborted and will not run", t.Id)
		grip.Infof(abortMsg)
		return gimlet.NewJSONResponse(&apimodels.EndTaskResponse{})
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

	// Disable hosts and prevent them from performing more work if they have
	// system failed many tasks in a row.
	if event.AllRecentHostEventsAreSystemFailed(ctx, currentHost.Id, currentHost.ProvisionTime, consecutiveSystemFailureThreshold) {
		msg := fmt.Sprintf("host encountered %d consecutive system failures", consecutiveSystemFailureThreshold)
		grip.Error(message.WrapError(units.HandlePoisonedHost(ctx, h.env, currentHost, msg), message.Fields{
			"message": "unable to disable poisoned host",
			"host":    currentHost.Id,
		}))

		endTaskResp.ShouldExit = true
	}

	msg := message.Fields{
		"message":     "Successfully marked task as finished",
		"task_id":     t.Id,
		"execution":   t.Execution,
		"operation":   "mark end",
		"duration":    time.Since(finishTime),
		"should_exit": endTaskResp.ShouldExit,
		"status":      t.Status,
		"path":        fmt.Sprintf("/rest/v2/hosts/%s/task/%s/end", currentHost.Id, t.Id),
	}

	if t.IsPartOfDisplay() {
		msg["display_task_id"] = t.DisplayTaskId
	}

	grip.Info(msg)
	return gimlet.NewJSONResponse(endTaskResp)
}

type agentExitParams struct {
	host       *host.Host
	remoteAddr string
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
		if err := params.host.StopAgentMonitor(ctx, env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor for quarantine",
				"host_id":       params.host.Id,
				"host_status":   params.host.Status,
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": params.host.AgentRevision,
			}))
		}
		if err := params.host.SetNeedsAgentDeploy(ctx, true); err != nil {
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
			grip.Warning(msg)
		}
		return true, nil
	case evergreen.HostDecommissioned:
		return true, nil
	default:
		return false, nil
	}
}
