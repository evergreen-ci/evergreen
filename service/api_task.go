package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

// if a host encounters more than this number of system failures, then it should be disabled.
const consecutiveSystemFailureThreshold = 3
const taskQueueServiceTTL = time.Minute

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
	if err = util.ReadJSONInto(util.NewRequestReader(r), taskStartInfo); err != nil {
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
	if idleTimeStartAt.IsZero() || idleTimeStartAt == util.ZeroTime {
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

// validateTaskEndDetails returns true if the task is finished or undispatched
func validateTaskEndDetails(details *apimodels.TaskEndDetail) bool {
	return details.Status == evergreen.TaskSucceeded ||
		details.Status == evergreen.TaskFailed ||
		details.Status == evergreen.TaskUndispatched
}

// checkHostHealth checks that host is running.
func checkHostHealth(h *host.Host) bool {
	if h.Status != evergreen.HostRunning {
		grip.Info(message.Fields{
			"message": "host is not running, so agent should exit",
			"host_id": h.Id,
		})
		return true
	}
	return false
}

// agentRevisionIsOld checks that the agent revision is current.
func agentRevisionIsOld(h *host.Host) bool {
	if h.AgentRevision != evergreen.BuildRevision {
		grip.InfoWhen(h.LegacyBootstrap(), message.Fields{
			"message":        "agent has wrong revision, so it should exit",
			"host_revision":  h.AgentRevision,
			"agent_revision": evergreen.BuildRevision,
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
	const slowThreshold = 1 * time.Second
	finishTime := time.Now()

	t := MustHaveTask(r)
	currentHost := MustHaveHost(r)

	details := &apimodels.TaskEndDetail{}
	endTaskResp := &apimodels.EndTaskResponse{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if !validateTaskEndDetails(details) {
		msg := fmt.Errorf("Invalid end status '%v' for task %v", details.Status, t.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	if currentHost.RunningTask == "" {
		grip.Error(message.Fields{
			"message":                 "host is not assigned task, not clearing, asking agent to exit",
			"task_id":                 t.Id,
			"task_status_from_db":     t.Status,
			"task_details_from_db":    t.Details,
			"task_details_from_agent": details,
			"host_id":                 currentHost.Id,
		})
		endTaskResp.ShouldExit = true
		gimlet.WriteJSON(w, endTaskResp)
		return
	}

	// clear the running task on the host startPhaseAt that the task has finished
	if err := currentHost.ClearRunningAndSetLastTask(t); err != nil {
		err = errors.Wrapf(err, "error clearing running task %s for host %s", t.Id, currentHost.Id)
		grip.Errorf(err.Error())
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	projectRef, err := model.FindOneProjectRef(t.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("empty projectRef for task"))
		return
	}

	// mark task as finished
	updates := model.StatusChanges{}
	err = model.MarkEnd(t, APIServerLockTitle, finishTime, details, projectRef.DeactivatePrevious, &updates)
	if err != nil {
		err = errors.Wrapf(err, "Error calling mark finish on task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// the task was aborted if it is still in undispatched.
	// the active state should be inactive.
	if details.Status == evergreen.TaskUndispatched {
		if t.Activated {
			grip.Warningf("task %v is active and undispatched after being marked as finished", t.Id)
			return
		}
		message := fmt.Sprintf("task %v has been aborted and will not run", t.Id)
		grip.Infof(message)
		endTaskResp = &apimodels.EndTaskResponse{}
		gimlet.WriteJSON(w, endTaskResp)
		return
	}

	// For a single-host task group, if a task fails, block and dequeue later tasks in that group.
	if t.TaskGroup != "" && t.TaskGroupMaxHosts == 1 && details.Status != evergreen.TaskSucceeded {
		if err = model.BlockTaskGroupTasks(t.Id); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem blocking task group tasks",
				"task_id": t.Id,
			}))
		}
		grip.Debug(message.Fields{
			"message": "blocked task group tasks for task",
			"task_id": t.Id,
		})
	}

	job := units.NewCollectTaskEndDataJob(t, currentHost)
	if err = as.queue.Put(r.Context(), job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "couldn't queue job to update task cost accounting"))
		return
	}

	// update the bookkeeping entry for the task
	err = task.UpdateExpectedDuration(t, t.TimeTaken)
	if err != nil {
		grip.Warning(message.WrapError(err, "problem updating expected duration"))
	}

	env := evergreen.GetEnvironment()
	if checkHostHealth(currentHost) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "problem stopping agent monitor",
				"host":      currentHost.Id,
				"operation": "next_task",
				"revision":  evergreen.BuildRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
		if err = currentHost.SetNeedsAgentDeploy(true); err != nil {
			grip.Error(message.WrapErrorf(err, "error indicating host %s needs deploy", currentHost.Id))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
		endTaskResp.ShouldExit = true
	}

	// we should disable hosts and prevent them from performing
	// more work if they appear to be in a bad state
	// (e.g. encountered 5 consecutive system failures)
	if event.AllRecentHostEventsMatchStatus(currentHost.Id, consecutiveSystemFailureThreshold, evergreen.TaskSystemFailed) {
		msg := "host encountered consecutive system failures"
		if currentHost.Provider != evergreen.ProviderNameStatic {
			err = currentHost.DisablePoisonedHost(msg)

			job := units.NewDecoHostNotifyJob(env, currentHost, err, msg)
			grip.Critical(message.WrapError(as.queue.Put(r.Context(), job),
				message.Fields{
					"host_id": currentHost.Id,
					"task_id": t.Id,
				}))

			if err != nil {
				gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "problem stopping agent monitor",
				"host":      currentHost.Id,
				"operation": "next_task",
				"revision":  evergreen.BuildRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
		endTaskResp.ShouldExit = true
	}

	grip.Info(message.Fields{
		"message":   "Successfully marked task as finished",
		"task_id":   t.Id,
		"execution": t.Execution,
		"operation": "mark end",
		"duration":  time.Since(finishTime),
	})
	gimlet.WriteJSON(w, endTaskResp)
}

// assignNextAvailableTask gets the next task from the queue and sets the running task field
// of currentHost.
func assignNextAvailableTask(taskQueue *model.TaskQueue, taskQueueService model.TaskQueueService, currentHost *host.Host) (*task.Task, error) {
	if currentHost.RunningTask != "" {
		grip.Error(message.Fields{
			"message":      "tried to assign task to a host already running task",
			"running_task": currentHost.RunningTask,
		})
		return nil, errors.New("cannot assign a task to a host with a running task")
	}

	var spec model.TaskSpec
	if currentHost.LastTask != "" {
		spec = model.TaskSpec{
			Group:        currentHost.LastGroup,
			BuildVariant: currentHost.LastBuildVariant,
			ProjectID:    currentHost.LastProject,
			Version:      currentHost.LastVersion,
		}
	}

	d, err := distro.FindOne(distro.ById(currentHost.Distro.Id))
	if err != nil {
		// Should we bailout if there is a database error leaving us unsure if the distro document actual exists?
		m := "database error while retrieving distro document;"
		if adb.ResultsNotFound(err) {
			m = "cannot find the db.distro document for the given distro;"
		}
		grip.Warning(message.Fields{
			"message":   m + " falling back to host.Distro",
			"distro_id": currentHost.Distro.Id,
			"host_id":   currentHost.Id,
		})
		d = currentHost.Distro
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
		var queueItem *model.TaskQueueItem
		switch d.PlannerSettings.Version {
		case evergreen.PlannerVersionTunable, evergreen.PlannerVersionRevised:
			queueItem, err = taskQueueService.RefreshFindNextTask(d.Id, spec)
			if err != nil {
				grip.Critical(message.WrapError(err, message.Fields{
					"message":                  "problem getting next task for the given host",
					"distro_id":                d.Id,
					"host_id":                  currentHost.Id,
					"host_last_task_id":        currentHost.LastTask,
					"taskspec_group":           spec.Group,
					"taskspec_build_variant":   spec.BuildVariant,
					"taskspec_version":         spec.Version,
					"taskspec_project_id":      spec.ProjectID,
					"taskspec_group_max_hosts": spec.GroupMaxHosts,
				}))
				return nil, errors.Wrap(err, "problem getting next task")
			}
		default:
			queueItem = taskQueue.FindNextTask(spec)
		}
		if queueItem == nil {
			grip.DebugWhen(d.PlannerSettings.Version == evergreen.PlannerVersionRevised, message.Fields{
				// "ticket":                        "EVG-6289",
				"function":                      "assignNextAvailableTask",
				"message":                       "taskQueueService.RefreshFindNextTask returned no task - returning nil",
				"distro_id":                     d.Id,
				"host_id":                       currentHost.Id,
				"host_last_task_id":             currentHost.LastTask,
				"host_last_group":               currentHost.LastGroup,
				"host_last_build_variant":       currentHost.LastBuildVariant,
				"host_last_version":             currentHost.LastVersion,
				"host_last_project":             currentHost.LastProject,
				"host_task_count":               currentHost.TaskCount,
				"host_last_task_completed_time": currentHost.LastTaskCompletedTime,
				"taskspec_group":                spec.Group,
				"taskspec_build_variant":        spec.BuildVariant,
				"taskspec_version":              spec.Version,
				"taskspec_project_id":           spec.ProjectID,
				"taskspec_group_max_hosts":      spec.GroupMaxHosts,
				"task_queue_length":             taskQueue.Length(),
			})

			return nil, nil
		}

		nextTask, err := task.FindOneNoMerge(task.ById(queueItem.Id))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":                  "database error while retrieving the db.tasks document for the next task to be assigned to this host",
				"distro_id":                d.Id,
				"host_id":                  currentHost.Id,
				"next_task_id":             queueItem.Id,
				"last_task_id":             currentHost.LastTask,
				"taskspec_group":           spec.Group,
				"taskspec_build_variant":   spec.BuildVariant,
				"taskspec_version":         spec.Version,
				"taskspec_project_id":      spec.ProjectID,
				"taskspec_group_max_hosts": spec.GroupMaxHosts,
			}))
			return nil, err
		}

		if nextTask == nil {
			grip.Error(message.Fields{
				"message":                  "cannot find a db.tasks document for the next task to be assigned to this host",
				"distro_id":                d.Id,
				"host_id":                  currentHost.Id,
				"next_task_id":             queueItem.Id,
				"last_task_id":             currentHost.LastTask,
				"taskspec_group":           spec.Group,
				"taskspec_build_variant":   spec.BuildVariant,
				"taskspec_version":         spec.Version,
				"taskspec_project_id":      spec.ProjectID,
				"taskspec_group_max_hosts": spec.GroupMaxHosts,
			})

			// An error is not returned in this situation due to https://jira.mongodb.org/browse/EVG-6214
			return nil, nil
		}

		// validate that the task can be run, if not fetch the next one in the queue.
		if !nextTask.IsDispatchable() {
			grip.Warning(message.Fields{
				"message":                  "skipping un-dispatchable task",
				"distro_id":                d.Id,
				"task_id":                  nextTask.Id,
				"status":                   nextTask.Status,
				"activated":                nextTask.Activated,
				"host_id":                  currentHost.Id,
				"taskspec_group":           spec.Group,
				"taskspec_build_variant":   spec.BuildVariant,
				"taskspec_version":         spec.Version,
				"taskspec_project_id":      spec.ProjectID,
				"taskspec_group_max_hosts": spec.GroupMaxHosts,
			})

			// Dequeue the task so we don't get it on another iteration of the loop.
			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":                  "nextTask.IsDispatchable() is false, but there was an issue dequeuing the task",
				"distro_id":                d.Id,
				"task_id":                  nextTask.Id,
				"host_id":                  currentHost.Id,
				"taskspec_group":           spec.Group,
				"taskspec_build_variant":   spec.BuildVariant,
				"taskspec_version":         spec.Version,
				"taskspec_project_id":      spec.ProjectID,
				"taskspec_group_max_hosts": spec.GroupMaxHosts,
			}))

			continue
		}

		projectRef, err := model.FindOneProjectRef(nextTask.Project)
		if err != nil || projectRef == nil {
			grip.Alert(message.Fields{
				"task_id": nextTask.Id,
				"message": "could not find project ref for next task, skipping",
				"project": nextTask.Project,
				"host_id": currentHost.Id,
			})
			return nil, errors.Wrapf(err, "could not find project ref for next task %s", nextTask.Id)
		}

		if !projectRef.Enabled {
			grip.Warning(message.Fields{
				"task_id": nextTask.Id,
				"project": nextTask.Project,
				"host_id": currentHost.Id,
				"message": "skipping task because of disabled project",
			})

			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":                  "projectRef.Enabled is false, but there was an issue dequeuing the task",
				"distro_id":                nextTask.DistroId,
				"task_id":                  nextTask.Id,
				"host_id":                  currentHost.Id,
				"taskspec_group":           spec.Group,
				"taskspec_build_variant":   spec.BuildVariant,
				"taskspec_version":         spec.Version,
				"taskspec_project_id":      spec.ProjectID,
				"taskspec_group_max_hosts": spec.GroupMaxHosts,
			}))

			continue
		}

		// UpdateRunningTask updates the running task in the host document
		ok, err := currentHost.UpdateRunningTask(nextTask)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// Dequeue the task so we don't get it on another iteration of the loop.
		grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
			"message":                  "updated the relevant running task fields for the given host, but there was an issue dequeuing the task",
			"distro_id":                nextTask.DistroId,
			"task_id":                  nextTask.Id,
			"host_id":                  currentHost.Id,
			"taskspec_group":           spec.Group,
			"taskspec_build_variant":   spec.BuildVariant,
			"taskspec_version":         spec.Version,
			"taskspec_project_id":      spec.ProjectID,
			"taskspec_group_max_hosts": spec.GroupMaxHosts,
		}))

		if !ok {
			continue
		}

		return nextTask, nil
	}
	return nil, nil
}

// NextTask retrieves the next task's id given the host name and host secret by retrieving the task queue
// and popping the next task off the task queue.
func (as *APIServer) NextTask(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	h := MustHaveHost(r)

	// stopAgentMonitor is only used for debug log purposes.
	var stopAgentMonitor bool
	defer func() {
		grip.DebugWhen(time.Since(begin) > time.Second, message.Fields{
			"message":            "slow next_task operation",
			"host_id":            h.Id,
			"distro":             h.Distro.Id,
			"latency":            time.Since(begin),
			"stop_agent_monitor": stopAgentMonitor,
		})
	}()

	var response apimodels.NextTaskResponse
	var err error
	if checkHostHealth(h) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		env := evergreen.GetEnvironment()
		stopAgentMonitor = true
		if err = h.StopAgentMonitor(ctx, env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "problem stopping agent monitor",
				"host":      h.Id,
				"operation": "next_task",
				"revision":  evergreen.BuildRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}

		if err = h.SetNeedsAgentDeploy(true); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host":      h.Id,
				"operation": "next_task",
				"message":   "problem indicating that host needs new agent or agent monitor deploy",
				"source":    "database error",
				"revision":  evergreen.BuildRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
		response.ShouldExit = true
		gimlet.WriteJSON(w, response)
		return
	}

	var agentExit bool
	response, agentExit = handleOldAgentRevision(response, h, w, r)
	if agentExit {
		return
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		err = errors.Wrap(err, "error retrieving admin settings")
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
	}
	if flags.TaskDispatchDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), "task dispatch is disabled, returning no task")
		gimlet.WriteJSON(w, response)
		return
	}

	// if there is already a task assigned to the host send back that task
	if h.RunningTask != "" {
		sendBackRunningTask(h, response, w)
		return
	}

	// retrieve the next task off the task queue and attempt to assign it to the host.
	// If there is already a host that has the task, it will error
	taskQueue, err := model.LoadTaskQueue(h.Distro.Id)
	if err != nil {
		err = errors.Wrapf(err, "Error locating distro queue (%v) for host '%v'", h.Distro.Id, h.Id)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}
	if taskQueue == nil {
		grip.Info(message.Fields{
			"message":   "nil task queue found",
			"op":        "next_task",
			"host_id":   h.Id,
			"distro_id": h.Distro.Id,
		})
		gimlet.WriteJSON(w, response)
		return
	}
	// assign the task to a host and retrieve the task
	nextTask, err := assignNextAvailableTask(taskQueue, as.taskQueueService, h)
	if err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
		return
	}
	if nextTask == nil {
		// if we couldn't find a task in the task queue,
		// check the alias queue...
		aliasQueue, err := model.LoadDistroAliasTaskQueue(h.Distro.Id)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
			return
		}
		nextTask, err = assignNextAvailableTask(aliasQueue, as.taskAliasQueueService, h)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
			return
		}

		if nextTask == nil {
			// if the task is empty, still send it with an status ok and check it on the other side
			grip.Info(message.Fields{
				"op":      "next_task",
				"message": "no task to assign to host",
				"host_id": h.Id,
			})
			gimlet.WriteJSON(w, response)
			return
		}
	}

	// mark the task as dispatched
	if err := model.MarkTaskDispatched(nextTask, h.Id, h.Distro.Id); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}
	setNextTask(nextTask, &response)
	gimlet.WriteJSON(w, response)
}

func handleOldAgentRevision(response apimodels.NextTaskResponse, h *host.Host, w http.ResponseWriter, r *http.Request) (apimodels.NextTaskResponse, bool) {
	if agentRevisionIsOld(h) {
		details := &apimodels.GetNextTaskDetails{}
		if err := util.ReadJSONInto(util.NewRequestReader(r), details); err != nil {
			if innerErr := h.SetNeedsNewAgent(true); innerErr != nil {
				grip.Error(message.WrapError(innerErr, message.Fields{
					"host":      h.Id,
					"operation": "next_task",
					"message":   "problem indicating that host needs new agent",
					"source":    "database error",
					"revision":  evergreen.BuildRevision,
				}))
				gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(innerErr))
				return apimodels.NextTaskResponse{}, true
			}
			grip.Info(message.WrapError(err, message.Fields{
				"host":          h.Id,
				"operation":     "next_task",
				"message":       "unable to unmarshal next task details, so updating agent",
				"host_revision": h.AgentRevision,
				"revision":      evergreen.BuildRevision,
			}))
			response.ShouldExit = true
			gimlet.WriteJSON(w, response)
			return apimodels.NextTaskResponse{}, true
		}

		// Non-legacy hosts deploying agents via the agent monitor may be
		// running an agent on the current revision, but the database host has
		// yet to be updated.
		if !h.LegacyBootstrap() && details.AgentRevision != h.AgentRevision {
			err := h.SetAgentRevision(details.AgentRevision)
			if err == nil {
				event.LogHostAgentDeployed(h.Id)
				return response, false
			}
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "problem updating host agent revision",
				"operation":     "next_task",
				"host":          h.Id,
				"source":        "database error",
				"host_revision": details.AgentRevision,
				"revsision":     evergreen.BuildRevision,
			}))
		}

		if details.TaskGroup == "" {
			if err := h.SetNeedsNewAgent(true); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"host":      h.Id,
					"operation": "next_task",
					"message":   "problem indicating that host needs new agent",
					"source":    "database error",
					"revision":  evergreen.BuildRevision,
				}))
				gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
				return apimodels.NextTaskResponse{}, true

			}
			if err := h.ClearRunningTask(); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"host":      h.Id,
					"operation": "next_task",
					"message":   "problem unsetting running task",
					"source":    "database error",
					"revision":  evergreen.BuildRevision,
				}))
				gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
				return apimodels.NextTaskResponse{}, true
			}
			response.ShouldExit = true
			gimlet.WriteJSON(w, response)
			return apimodels.NextTaskResponse{}, true
		}
	}
	return response, false
}

func sendBackRunningTask(h *host.Host, response apimodels.NextTaskResponse, w http.ResponseWriter) {
	var err error
	var t *task.Task
	t, err = task.FindOne(task.ById(h.RunningTask))
	if err != nil {
		err = errors.Wrapf(err, "error getting running task %s", h.RunningTask)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}

	// if the task can be dispatched and activated dispatch it
	if t.IsDispatchable() {
		err = errors.WithStack(model.MarkTaskDispatched(t, h.Id, h.Distro.Id))
		if err != nil {
			grip.Error(errors.Wrapf(err, "error while marking task %s as dispatched for host %s", t.Id, h.Id))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return
		}
	}
	// if the task is activated return that task
	if t.Activated {
		setNextTask(t, &response)
		gimlet.WriteJSON(w, response)
		return
	}
	// the task is not activated so the host's running task should be unset
	// so it can retrieve a new task.
	if err = h.ClearRunningTask(); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}

	// return an empty
	grip.Info(message.Fields{
		"op":      "next_task",
		"message": "unset running task field for inactive task on host",
		"host_id": h.Id,
		"task_id": t.Id,
	})
	gimlet.WriteJSON(w, response)
	return
}

func setNextTask(t *task.Task, response *apimodels.NextTaskResponse) {
	response.TaskId = t.Id
	response.TaskSecret = t.Secret
	response.TaskGroup = t.TaskGroup
	response.Version = t.Version
	response.Build = t.BuildId
}
