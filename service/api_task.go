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
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
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
	if err = utility.ReadJSON(util.NewRequestReader(r), taskStartInfo); err != nil {
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

// validateTaskEndDetails returns true if the task is finished or undispatched
func validateTaskEndDetails(details *apimodels.TaskEndDetail) bool {
	return details.Status == evergreen.TaskSucceeded ||
		details.Status == evergreen.TaskFailed ||
		details.Status == evergreen.TaskUndispatched
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
	const slowThreshold = 1 * time.Second
	finishTime := time.Now()

	t := MustHaveTask(r)
	currentHost := MustHaveHost(r)

	details := &apimodels.TaskEndDetail{}
	endTaskResp := &apimodels.EndTaskResponse{}
	if err := utility.ReadJSON(util.NewRequestReader(r), details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if !validateTaskEndDetails(details) {
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

	// clear the running task on the host startPhaseAt that the task has finished
	if err := currentHost.ClearRunningAndSetLastTask(t); err != nil {
		err = errors.Wrapf(err, "error clearing running task %s for host %s", t.Id, currentHost.Id)
		grip.Errorf(err.Error())
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	projectRef, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("empty projectRef for task"))
		return
	}

	// For a single-host task group, if a task fails, block and dequeue later tasks in that group.
	// Call before MarkEnd so the version is marked finished when this is the last task in the version
	// to finish
	if t.IsPartOfSingleHostTaskGroup() && details.Status != evergreen.TaskSucceeded {
		// BlockTaskGroups is a recursive operation, which
		// includes updating a large number of task
		// documents.
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

	// mark task as finished
	deactivatePrevious := utility.FromBoolPtr(projectRef.DeactivatePrevious)
	err = model.MarkEnd(t, APIServerLockTitle, finishTime, details, deactivatePrevious)
	if err != nil {
		err = errors.Wrapf(err, "Error calling mark finish on task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if t.Requester == evergreen.MergeTestRequester && details.Status != evergreen.TaskSucceeded && !t.Aborted {
		if err = model.DequeueAndRestart(t, APIServerLockTitle, fmt.Sprintf("task '%s' failed", t.DisplayName)); err != nil {
			err = errors.Wrapf(err, "Error dequeueing and aborting failed commit queue version")
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
		message := fmt.Sprintf("task %v has been aborted and will not run", t.Id)
		grip.Infof(message)
		endTaskResp = &apimodels.EndTaskResponse{}
		gimlet.WriteJSON(w, endTaskResp)
		return
	}

	// GetDisplayTask will set the DisplayTask on t if applicable
	// we set this before the collect task end job is run to prevent data race
	dt, err := t.GetDisplayTask()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}
	job := units.NewCollectTaskEndDataJob(t, currentHost)
	if err = as.queue.Put(r.Context(), job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "couldn't queue job to update task stats accounting"))
		return
	}

	if checkHostHealth(currentHost) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, as.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor",
				"host_id":       currentHost.Id,
				"operation":     "next_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": currentHost.AgentRevision,
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
			grip.Error(message.WrapError(units.HandlePoisonedHost(r.Context(), as.env, currentHost, msg), message.Fields{
				"message": "unable to disable poisoned host",
				"host":    currentHost.Id,
			}))
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = currentHost.StopAgentMonitor(ctx, as.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor",
				"host_id":       currentHost.Id,
				"operation":     "next_task",
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
	}

	if dt != nil {
		msg["display_task_id"] = t.DisplayTask.Id
	}

	grip.Info(msg)
	gimlet.WriteJSON(w, endTaskResp)
}

// prepareForReprovision readies host for reprovisioning.
func prepareForReprovision(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	if err := h.MarkAsReprovisioning(); err != nil {
		return errors.Wrap(err, "error marking host as ready for reprovisioning")
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

	d, err := distro.FindOne(distro.ById(currentHost.Distro.Id))
	if err != nil {
		// Should we bailout if there is a database error leaving us unsure if the distro document actually exists?
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
	grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
		"message":     "assignNextAvailableTask performance",
		"step":        "distro.FindOne",
		"duration_ns": time.Now().Sub(stepStart),
		"run_id":      runId,
	})
	stepStart = time.Now()

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
			queueItem, err = dispatcher.RefreshFindNextTask(d.Id, spec)
			if err != nil {
				return nil, false, errors.Wrap(err, "problem getting next task")
			}
		default:
			queueItem, _ = taskQueue.FindNextTask(spec)
		}

		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "RefreshFindNextTask",
			"duration_ns": time.Now().Sub(stepStart),
			"run_id":      runId,
		})
		stepStart = time.Now()
		if queueItem == nil {
			return nil, false, nil
		}

		nextTask, err := task.FindOne(task.ById(queueItem.Id))
		if err != nil {
			grip.DebugWhen(queueItem.Group != "", message.Fields{
				"message":            "error retrieving next task",
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
			"duration_ns": time.Now().Sub(stepStart),
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
		if !nextTask.IsDispatchable() {
			// Dequeue the task so we don't get it on another iteration of the loop.
			grip.Warning(message.WrapError(taskQueue.DequeueTask(nextTask.Id), message.Fields{
				"message":   "nextTask.IsDispatchable() is false, but there was an issue dequeuing the task",
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
			"duration_ns": time.Now().Sub(stepStart),
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
			"duration_ns": time.Now().Sub(stepStart),
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
						if tgTask.TaskGroupOrder < nextTask.TaskGroupOrder && tgTask.IsDispatchable() && !tgTask.Blocked() {
							dispatchRace = fmt.Sprintf("an earlier task ('%s') in the task group is still dispatchable", tgTask.DisplayName)
						}
					}
				}
			}
			grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
				"message":     "assignNextAvailableTask performance",
				"step":        "find task group",
				"duration_ns": time.Now().Sub(stepStart),
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
				"duration_ns": time.Now().Sub(stepStart),
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
				"duration_ns": time.Now().Sub(stepStart),
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
			"duration_ns": time.Now().Sub(stepStart),
			"run_id":      runId,
		})
		grip.DebugWhen(currentHost.Distro.Id == distroToMonitor, message.Fields{
			"message":     "assignNextAvailableTask performance",
			"step":        "total",
			"duration_ns": time.Now().Sub(funcStart),
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

// NextTask retrieves the next task's id given the host name and host secret by retrieving the task queue
// and popping the next task off the task queue.
func (as *APIServer) NextTask(w http.ResponseWriter, r *http.Request) {
	begin := time.Now()
	h := MustHaveHost(r)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if h.AgentStartTime.IsZero() {
		if err := h.SetAgentStartTime(); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "could not set host's agent start time for first contact",
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			}))
		} else {
			grip.InfoWhen(h.Provider != evergreen.ProviderNameStatic, message.Fields{
				"message":                   "agent initiated first contact with server",
				"host_id":                   h.Id,
				"distro":                    h.Distro.Id,
				"provisioning":              h.Distro.BootstrapSettings.Method,
				"agent_start_duration_secs": time.Since(h.CreationTime).Seconds(),
			})
		}
	}

	grip.Error(message.WrapError(h.SetUserDataHostProvisioned(), message.Fields{
		"message":      "failed to mark host as done provisioning with user data",
		"host_id":      h.Id,
		"distro":       h.Distro.Id,
		"provisioning": h.Distro.BootstrapSettings.Method,
		"operation":    "next_task",
	}))

	stoppedAgentMonitor := (h.Distro.LegacyBootstrap() && h.NeedsReprovision == host.ReprovisionToLegacy ||
		h.NeedsReprovision == host.ReprovisionRestartJasper)
	defer func() {
		grip.DebugWhen(time.Since(begin) > time.Second, message.Fields{
			"message":               "slow next_task operation",
			"host_id":               h.Id,
			"distro":                h.Distro.Id,
			"latency":               time.Since(begin).Seconds(),
			"stopped_agent_monitor": stoppedAgentMonitor,
		})
	}()

	var response apimodels.NextTaskResponse
	if responded := handleReprovisioning(ctx, as.env, h, response, w); responded {
		return
	}

	var err error
	if checkHostHealth(h) {
		response.ShouldExit = true

		ctx, cancel = context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err = h.StopAgentMonitor(ctx, as.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":       "problem stopping agent monitor",
				"host_id":       h.Id,
				"operation":     "next_task",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": h.AgentRevision,
			}))
			gimlet.WriteJSON(w, response)
			return
		}
		if err = h.SetNeedsAgentDeploy(true); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":       h.Id,
				"operation":     "next_task",
				"message":       "problem indicating that host needs new agent or agent monitor deploy",
				"source":        "database error",
				"revision":      evergreen.BuildRevision,
				"agent":         evergreen.AgentVersion,
				"current_agent": h.AgentRevision,
			}))
			gimlet.WriteJSON(w, response)
			return
		}
		gimlet.WriteJSON(w, response)
		return
	}
	var agentExit bool
	details, agentExit := getDetails(response, h, w, r)
	if agentExit {
		return
	}
	response, agentExit = handleOldAgentRevision(response, details, h, w)
	if agentExit {
		return
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		err = errors.Wrap(err, "error retrieving admin settings")
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
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

	var nextTask *task.Task
	var shouldRunTeardown bool

	// retrieve the next task off the task queue and attempt to assign it to the host.
	// If there is already a host that has the task, it will error
	taskQueue, err := model.LoadTaskQueue(h.Distro.Id)
	if err != nil {
		err = errors.Wrapf(err, "Error locating distro queue (%v) for host '%v'", h.Distro.Id, h.Id)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}

	// if the task queue exists, try to assign a task from it:
	if taskQueue != nil {
		// assign the task to a host and retrieve the task
		nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, taskQueue, as.taskDispatcher, h, details)
		if err != nil {
			err = errors.WithStack(err)
			grip.Error(err)
			gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
			return
		}
	}

	// if we didn't find a task in the "primary" queue, then we
	// try again from the alias queue. (this code runs if the
	// primary queue doesn't exist or is empty)
	if nextTask == nil && !shouldRunTeardown {
		// if we couldn't find a task in the task queue,
		// check the alias queue...
		aliasQueue, err := model.LoadDistroAliasTaskQueue(h.Distro.Id)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
			return
		}
		if aliasQueue != nil {
			nextTask, shouldRunTeardown, err = assignNextAvailableTask(ctx, aliasQueue, as.taskAliasDispatcher, h, details)
			if err != nil {
				gimlet.WriteResponse(w, gimlet.MakeJSONErrorResponder(err))
				return
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
				"host_id": h.Id,
			})
			response.ShouldTeardownGroup = true
		} else {
			// if the task is empty, still send it with an status ok and check it on the other side
			grip.Info(message.Fields{
				"op":      "next_task",
				"message": "no task to assign to host",
				"host_id": h.Id,
			})
		}

		gimlet.WriteJSON(w, response)
		return
	}

	// otherwise we've dispatched a task, so we
	// mark the task as dispatched
	if err := model.MarkTaskDispatched(nextTask, h); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}
	setNextTask(nextTask, &response)
	gimlet.WriteJSON(w, response)
}

func getDetails(response apimodels.NextTaskResponse, h *host.Host, w http.ResponseWriter, r *http.Request) (*apimodels.GetNextTaskDetails, bool) {
	isOldAgent := agentRevisionIsOld(h)
	// if agent revision is old, we should indicate an exit if there are errors
	details := &apimodels.GetNextTaskDetails{}
	if err := utility.ReadJSON(util.NewRequestReader(r), details); err != nil {
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
				gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(innerErr))
				return nil, true
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
			response.ShouldExit = true
			gimlet.WriteJSON(w, response)
			return nil, true
		}
	}
	return details, false
}

func handleReprovisioning(ctx context.Context, env evergreen.Environment, h *host.Host, response apimodels.NextTaskResponse, w http.ResponseWriter) (responded bool) {
	if h.NeedsReprovision == host.ReprovisionNone {
		return false
	}
	if !utility.StringSliceContains([]string{evergreen.HostProvisioning, evergreen.HostRunning}, h.Status) {
		return false
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
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return true
	}
	response.ShouldExit = true
	gimlet.WriteJSON(w, response)
	return true
}

func handleOldAgentRevision(response apimodels.NextTaskResponse, details *apimodels.GetNextTaskDetails, h *host.Host, w http.ResponseWriter) (apimodels.NextTaskResponse, bool) {
	if !agentRevisionIsOld(h) {
		return response, false
	}

	// Non-legacy hosts deploying agents via the agent monitor may be
	// running an agent on the current revision, but the database host has
	// yet to be updated.
	if !h.Distro.LegacyBootstrap() && details.AgentRevision != h.AgentRevision {
		err := h.SetAgentRevision(details.AgentRevision)
		if err == nil {
			event.LogHostAgentDeployed(h.Id)
			return response, false
		}
		grip.Error(message.WrapError(err, message.Fields{
			"message":        "problem updating host agent revision",
			"operation":      "next_task",
			"host_id":        h.Id,
			"source":         "database error",
			"host_revision":  details.AgentRevision,
			"agent_version":  evergreen.AgentVersion,
			"build_revision": evergreen.BuildRevision,
		}))
	}

	if details.TaskGroup == "" {
		if err := h.SetNeedsNewAgent(true); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":        h.Id,
				"operation":      "next_task",
				"message":        "problem indicating that host needs new agent",
				"source":         "database error",
				"build_revision": evergreen.BuildRevision,
				"agent_version":  evergreen.AgentVersion,
				"host_revision":  h.AgentRevision,
			}))
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return apimodels.NextTaskResponse{}, true

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
			gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
			return apimodels.NextTaskResponse{}, true
		}
		response.ShouldExit = true
		gimlet.WriteJSON(w, response)
		return apimodels.NextTaskResponse{}, true
	}

	return response, false
}

func sendBackRunningTask(h *host.Host, response apimodels.NextTaskResponse, w http.ResponseWriter) {
	var err error
	var t *task.Task
	t, err = task.FindOneId(h.RunningTask)
	if err != nil {
		err = errors.Wrapf(err, "error getting running task %s", h.RunningTask)
		grip.Error(err)
		gimlet.WriteResponse(w, gimlet.MakeJSONInternalErrorResponder(err))
		return
	}

	// if the task can be dispatched and activated dispatch it
	if t.IsDispatchable() {
		err = errors.WithStack(model.MarkTaskDispatched(t, h))
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
