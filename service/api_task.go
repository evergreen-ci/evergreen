package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/bookkeeping"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

// if a host encounters more than this number of system failures, then it should be disabled.
const consecutiveSystemFailureThreshold = 3

// StartTask is the handler function that retrieves the task from the request
// and acquires the global lock
// With the lock, it marks associated tasks, builds, and versions as started.
// It then updates the host document with relevant information, including the pid
// of the agent, and ensures that the host has the running task field set.
func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	var err error

	t := MustHaveTask(r)
	grip.Infoln("Marking task started:", t.Id)

	taskStartInfo := &apimodels.TaskStartRequest{}
	if err = util.ReadJSONInto(util.NewRequestReader(r), taskStartInfo); err != nil {
		http.Error(w, fmt.Sprintf("Error reading task start request for %v: %v", t.Id, err), http.StatusBadRequest)
		return
	}

	updates := model.StatusChanges{}
	if err = model.MarkStart(t, &updates); err != nil {
		message := errors.Wrapf(err, "Error marking task '%s' started", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if t.Requester == evergreen.GithubPRRequester && updates.PatchNewStatus == evergreen.PatchStarted {
		job := units.NewGithubStatusUpdateJobForPatchWithVersion(t.Version)
		if err = as.queue.Put(job); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.New("error queuing github status api update"))
			return
		}
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

	job := units.NewCollectHostIdleDataJob(h, idleTimeStartAt, t.StartTime)
	if err = as.queue.Put(job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.New("error queuing host idle stats"))
		return
	}

	job = units.NewCollectTaskStartDataJob(t, h)
	if err = as.queue.Put(job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.New("error queuing task start stats"))
		return
	}

	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Task %v started on host %v", t.Id, h.Id))
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

// checkAgentRevision checks that the agent revision is current.
func checkAgentRevision(h *host.Host) bool {
	if h.AgentRevision != evergreen.BuildRevision {
		grip.Info(message.Fields{
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
// NOTE this should eventually become the default code path.
func (as *APIServer) EndTask(w http.ResponseWriter, r *http.Request) {
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
		message := fmt.Errorf("Error calling mark finish on task %v : %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	if t.Requester == evergreen.GithubPRRequester {
		if updates.BuildNewStatus == evergreen.BuildFailed || updates.BuildNewStatus == evergreen.BuildSucceeded {
			job := units.NewGithubStatusUpdateJobForBuild(t.BuildId)
			if err = as.queue.Put(job); err != nil {
				as.LoggedError(w, r, http.StatusInternalServerError, errors.New("couldn't queue job to update github status"))
				return
			}
		}

		if updates.PatchNewStatus == evergreen.PatchFailed || updates.PatchNewStatus == evergreen.PatchSucceeded {
			job := units.NewGithubStatusUpdateJobForPatchWithVersion(t.Version)
			if err = as.queue.Put(job); err != nil {
				as.LoggedError(w, r, http.StatusInternalServerError, errors.New("couldn't queue job to update github status"))
				return
			}
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
		as.WriteJSON(w, http.StatusOK, endTaskResp)
		return
	}

	// clear the running task on the host now that the task has finished
	if err = currentHost.ClearRunningTask(t.Id, time.Now()); err != nil {
		message := fmt.Errorf("error clearing running task %s for host %s : %v", t.Id, currentHost.Id, err)
		grip.Errorf(message.Error())
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	job := units.NewCollectTaskEndDataJob(t, currentHost)
	if err = as.queue.Put(job); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "couldn't queue job to update task cost accounting"))
		return
	}

	if !evergreen.IsPatchRequester(t.Requester) {
		if t.IsPartOfDisplay() {
			parent := t.DisplayTask
			if task.IsFinished(*parent) {
				grip.Error(errors.Wrapf(alerts.RunTaskFailureTriggers(parent.Id),
					"processing alert triggers for display task %s", parent.Id))
			}
		} else {
			grip.Infoln("Processing alert triggers for task", t.Id)

			grip.Error(errors.Wrapf(alerts.RunTaskFailureTriggers(t.Id),
				"processing alert triggers for task %s", t.Id))
		}
	}
	// TODO(EVG-223) process patch-specific triggers

	// update the bookkeeping entry for the task
	err = bookkeeping.UpdateExpectedDuration(t, t.TimeTaken)
	if err != nil {
		grip.Errorln("Error updating expected duration:", err)
	}

	if checkHostHealth(currentHost) {
		// set the needs new agent flag on the host
		if err := currentHost.SetNeedsNewAgent(true); err != nil {
			grip.Errorf("error indicating host %s needs new agent: %+v", currentHost.Id, err)
			as.WriteJSON(w, http.StatusInternalServerError, err)
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
			err := currentHost.DisablePoisonedHost()
			env := evergreen.GetEnvironment()

			job := units.NewDecoHostNotifyJob(env, currentHost, err, msg)
			grip.Critical(message.WrapError(as.queue.Put(job),
				message.Fields{
					"host_id": currentHost.Id,
					"task_id": t.Id,
				}))

			if err != nil {
				as.WriteJSON(w, http.StatusInternalServerError, err)
				return
			}
		}
		endTaskResp.ShouldExit = true
	}

	grip.Infof("Successfully marked task %s as finished", t.Id)
	as.WriteJSON(w, http.StatusOK, endTaskResp)
}

// assignNextAvailableTask gets the next task from the queue and sets the running task field
// of currentHost.
func assignNextAvailableTask(taskQueue model.TaskQueueAccessor, currentHost *host.Host) (*task.Task, error) {
	if currentHost.RunningTask != "" {
		grip.Error(message.Fields{
			"message":      "tried to assign task to a host already running task",
			"running_task": currentHost.RunningTask,
		})
		return nil, errors.New("cannot assign a task to a host with a running task")
	}

	var spec model.TaskSpec
	if currentHost.LastTaskCompleted != "" {
		t, err := task.FindOneId(currentHost.LastTaskCompleted)
		if err != nil {
			return nil, errors.Wrap(err, "error finding last task")
		}
		spec = model.TaskSpec{
			Group:        t.TaskGroup,
			BuildVariant: t.BuildVariant,
			ProjectID:    t.Project,
			Version:      t.Version,
		}
	}
	for taskQueue.Length() != 0 {
		queueItem := taskQueue.FindNextTask(spec)
		if queueItem == nil {
			return nil, errors.New("no dispatchable task found in the queue")
		}

		nextTask, err := task.FindOne(task.ById(queueItem.Id))
		if err != nil {
			return nil, err
		}
		if nextTask == nil {
			return nil, errors.New("nil task on the queue")
		}

		// dequeue the task from the queue
		if err = taskQueue.DequeueTask(nextTask.Id); err != nil {
			return nil, errors.Wrapf(err,
				"error pulling task with id %v from queue for distro %v",
				nextTask.Id, nextTask.DistroId)
		}

		// validate that the task can be run, if not fetch the next one in
		// the queue.
		if !nextTask.IsDispatchable() {
			grip.Warning(message.Fields{
				"message":   "skipping un-dispatchable task",
				"task_id":   nextTask.Id,
				"status":    nextTask.Status,
				"activated": nextTask.Activated,
				"host":      currentHost.Id,
			})
			continue
		}

		projectRef, err := model.FindOneProjectRef(nextTask.Project)
		if err != nil || projectRef == nil {
			grip.Warning(message.Fields{
				"task_id": nextTask.Id,
				"message": "could not find project ref for next task, skipping",
				"project": nextTask.Project,
				"host":    currentHost.Id,
			})
			continue
		}

		if !projectRef.Enabled {
			grip.Warning(message.Fields{
				"task_id": nextTask.Id,
				"project": nextTask.Project,
				"host":    currentHost.Id,
				"message": "skipping task because of disabled project",
			})
			continue
		}

		ok, err := currentHost.UpdateRunningTask(currentHost.LastTaskCompleted, nextTask, time.Now())
		if err != nil {
			return nil, errors.WithStack(err)
		}
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
	h := MustHaveHost(r)
	response := apimodels.NextTaskResponse{}
	if checkHostHealth(h) {
		if err := h.SetNeedsNewAgent(true); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host":      h.Id,
				"operation": "next_task",
				"message":   "problem indicating that host needs new agent",
				"source":    "database error",
				"revision":  evergreen.BuildRevision,
			}))
			as.WriteJSON(w, http.StatusInternalServerError, err)
			return
		}
		response.ShouldExit = true
		as.WriteJSON(w, http.StatusOK, response)
		return
	}
	if checkAgentRevision(h) {
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
				as.WriteJSON(w, http.StatusInternalServerError, innerErr)
				return
			}
			grip.Info(message.WrapError(err, message.Fields{
				"host":          h.Id,
				"operation":     "next_task",
				"message":       "unable to unmarshal next task details, so updating agent",
				"host_revision": h.AgentRevision,
				"revision":      evergreen.BuildRevision,
			}))
			response.ShouldExit = true
			as.WriteJSON(w, http.StatusOK, response)
			return
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
				as.WriteJSON(w, http.StatusInternalServerError, err)
				return
			}
			if err := h.UnsetRunningTask(); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"host":      h.Id,
					"operation": "next_task",
					"message":   "problem unsetting running task",
					"source":    "database error",
					"revision":  evergreen.BuildRevision,
				}))
				as.WriteJSON(w, http.StatusInternalServerError, err)
				return
			}
			response.ShouldExit = true
			as.WriteJSON(w, http.StatusOK, response)
			return
		}
		response.NewAgent = true
	}

	adminSettings, err := evergreen.GetConfig()
	if err != nil {
		err = errors.Wrap(err, "error retrieving admin settings")
		grip.Error(err)
		as.WriteJSON(w, http.StatusInternalServerError, err)
	}
	if adminSettings.ServiceFlags.TaskDispatchDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), "task dispatch is disabled, returning no task")
		as.WriteJSON(w, http.StatusOK, response)
		return
	}

	// if there is already a task assigned to the host send back that task
	if h.RunningTask != "" {
		var t *task.Task
		t, err = task.FindOne(task.ById(h.RunningTask))
		if err != nil {
			err = errors.WithStack(err)
			grip.Error(err)
			as.WriteJSON(w, http.StatusInternalServerError,
				errors.Wrapf(err, "error getting running task %s", h.RunningTask))
			return
		}

		// if the task can be dispatched and activated dispatch it
		if t.IsDispatchable() {
			err = errors.WithStack(model.MarkTaskDispatched(t, h.Id, h.Distro.Id))
			if err != nil {
				grip.Error(err)
				as.WriteJSON(w, http.StatusInternalServerError,
					errors.Wrapf(err, "error while marking task %s as dispatched for host %s", t.Id, h.Id))
				return
			}
		}
		// if the task is activated return that task
		if t.Activated {
			setNextTask(t, &response)
			as.WriteJSON(w, http.StatusOK, response)
			return
		}
		// the task is not activated so the host's running task should be unset
		// so it can retrieve a new task.
		if err = h.ClearRunningTask(h.LastTaskCompleted, time.Now()); err != nil {
			err = errors.WithStack(err)
			grip.Error(err)
			as.WriteJSON(w, http.StatusInternalServerError, err)
			return
		}

		// return an empty
		grip.Infof("Unset running task field for inactive task %s on host %s", t.Id, h.Id)
		as.WriteJSON(w, http.StatusOK, response)
		return
	}

	// retrieve the next task off the task queue and attempt to assign it to the host.
	// If there is already a host that has the task, it will error
	taskQueue, err := model.LoadTaskQueue(h.Distro.Id)
	if err != nil {
		err = errors.Wrapf(err, "Error locating distro queue (%v) for host '%v'", h.Distro.Id, h.Id)
		grip.Error(err)
		as.WriteJSON(w, http.StatusBadRequest, err)
		return
	}
	if taskQueue == nil {
		msg := fmt.Sprintf("Nil task queue found for task '%v's distro queue - '%v'",
			h.Id, h.Distro.Id)
		grip.Info(msg)
		as.WriteJSON(w, http.StatusOK, response)
		return
	}
	// assign the task to a host and retrieve the task
	nextTask, err := assignNextAvailableTask(taskQueue, h)
	if err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		as.WriteJSON(w, http.StatusBadRequest, err)
		return
	}
	if nextTask == nil {
		// if the task is empty, still send it with an status ok and check it on the other side
		grip.Infof("no task to assign host %v", h.Id)
		as.WriteJSON(w, http.StatusOK, response)
		return
	}

	// mark the task as dispatched
	if err := model.MarkTaskDispatched(nextTask, h.Id, h.Distro.Id); err != nil {
		err = errors.WithStack(err)
		grip.Error(err)
		as.WriteJSON(w, http.StatusInternalServerError, err)
		return
	}
	setNextTask(nextTask, &response)
	grip.Infof("assigned task %s to host %s", nextTask.Id, h.Id)
	as.WriteJSON(w, http.StatusOK, response)
}

func setNextTask(t *task.Task, response *apimodels.NextTaskResponse) {
	response.TaskId = t.Id
	response.TaskSecret = t.Secret
	response.TaskGroup = t.TaskGroup
	response.Version = t.Version
}
