package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	heartbeatTimeoutThreshold             = 7 * time.Minute
	taskExecutionTimeoutJobName           = "task-execution-timeout"
	taskExecutionTimeoutPopulationJobName = "task-execution-timeout-populate"
	maxTaskExecutionTimeoutAttempts       = 10
)

func init() {
	registry.AddJobType(taskExecutionTimeoutJobName, func() amboy.Job {
		return makeTaskExecutionTimeoutMonitorJob()
	})
	registry.AddJobType(taskExecutionTimeoutPopulationJobName, func() amboy.Job {
		return makeTaskExecutionTimeoutPopulateJob()
	})
}

type taskExecutionTimeoutJob struct {
	Task string `bson:"task_id"`

	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeTaskExecutionTimeoutMonitorJob() *taskExecutionTimeoutJob {
	j := &taskExecutionTimeoutJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskExecutionTimeoutJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewTaskExecutionMonitorJob returns a job to check if a running task has
// failed to send a heartbeat recently. If it has timed out, it is cleaned up.
func NewTaskExecutionMonitorJob(taskID string, ts string) amboy.Job {
	j := makeTaskExecutionTimeoutMonitorJob()
	j.Task = taskID
	j.SetID(fmt.Sprintf("%s.%s.%s", taskExecutionTimeoutJobName, taskID, ts))
	j.SetEnqueueScopes(fmt.Sprintf("%s.%s", taskExecutionTimeoutJobName, taskID))
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(maxTaskExecutionTimeoutAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(time.Minute),
	})
	return j
}

func (j *taskExecutionTimeoutJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}
	env := evergreen.GetEnvironment()

	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message":   "monitor is disabled",
			"operation": j.Type().Name,
			"impact":    "skipping task heartbeat cleanup job",
			"mode":      "degraded",
		})
		return
	}

	t, err := task.FindOneId(j.Task)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "finding task '%s'", j.Task))
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task '%s' not found", j.Task))
		return
	}

	// if the task has heartbeat since this job was queued, let it run
	if t.LastHeartbeat.Add(heartbeatTimeoutThreshold).After(time.Now()) {
		return
	}

	msg := message.Fields{
		"job":       j.ID(),
		"operation": j.Type().Name,
		"id":        j.ID(),
		"task":      t.Id,
	}
	if t.HostId != "" {
		msg["host_id"] = t.HostId
	}

	err = cleanUpTimedOutTask(ctx, env, j.ID(), t)
	if err != nil {
		msg["message"] = "failed to clean up timed-out task"
		grip.Warning(message.WrapError(err, msg))
		j.AddRetryableError(err)
		return
	}

	msg["message"] = "successfully cleaned up timed-out task"
	grip.Debug(msg)
}

// cleanUpTimedOutTask cleans up a single host task that has exceeded the
// task heartbeat timeout.
func cleanUpTimedOutTask(ctx context.Context, env evergreen.Environment, id string, t *task.Task) error {
	host, err := host.FindOne(host.ById(t.HostId))
	if err != nil {
		return errors.Wrapf(err, "finding host '%s' for task '%s'", t.HostId, t.Id)
	}

	// if there's no relevant host and the task is not a display task, something went wrong
	if host == nil {
		if !t.DisplayOnly {
			grip.Error(message.Fields{
				"message":   "no entry found for host",
				"task":      t.Id,
				"host_id":   t.HostId,
				"operation": "cleanup timed out task",
			})
		}
		return errors.WithStack(t.MarkUnscheduled())
	}

	// For a single-host task group, if a task fails, block and dequeue later tasks in that group.
	if t.IsPartOfSingleHostTaskGroup() && t.Status != evergreen.TaskSucceeded {
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

	// if the host still has the task as its running task, clear it.
	if host.RunningTask == t.Id {
		// Check if the host was externally terminated. When the running task is
		// cleared on the host, an agent or agent monitor deploy might run,
		// which updates the LCT and prevents detection of external termination
		// until the deploy job runs out of retries.
		var terminated bool
		terminated, err = handleExternallyTerminatedHost(ctx, id, env, host)
		if err != nil {
			return errors.Wrapf(err, "checking host '%s 'with timed out task '%s' for external termination", host.Id, t.Id)
		}
		if terminated {
			return nil
		}
		if err = host.ClearRunningAndSetLastTask(t); err != nil {
			return errors.Wrapf(err, "clearing running task '%s' from host '%s'", t.Id, host.Id)
		}
	}

	detail := &apimodels.TaskEndDetail{
		Description: evergreen.TaskDescriptionHeartbeat,
		Type:        evergreen.CommandTypeSystem,
		TimedOut:    true,
		Status:      evergreen.TaskFailed,
	}

	// try to reset the task
	if t.IsPartOfDisplay() {
		dt, err := t.GetDisplayTask()
		if err != nil {
			return errors.Wrapf(err, "getting display task")
		}
		if err = dt.SetResetWhenFinished(); err != nil {
			return errors.Wrap(err, "marking display task for reset when finished")
		}
		return errors.Wrap(model.MarkEnd(t, evergreen.MonitorPackage, time.Now(), detail, false), "marking execution task ended")
	}

	return errors.Wrapf(model.TryResetTask(t.Id, evergreen.User, evergreen.MonitorPackage, detail), "trying to reset task '%s'", t.Id)
}

////////////////////////////////////////////////////////////////////////
//
// Population Job

type taskExecutionTimeoutPopulationJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeTaskExecutionTimeoutPopulateJob() *taskExecutionTimeoutPopulationJob {
	j := &taskExecutionTimeoutPopulationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskExecutionTimeoutPopulationJobName,
				Version: 0,
			},
		},
	}
	return j
}

func NewTaskExecutionMonitorPopulateJob(id string) amboy.Job {
	j := makeTaskExecutionTimeoutPopulateJob()
	j.SetID(fmt.Sprintf("%s.%s", j.Type().Name, id))
	return j
}

func (j *taskExecutionTimeoutPopulationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}
	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message":   "monitor is disabled",
			"operation": j.Type().Name,
			"impact":    "skipping task heartbeat cleanup job",
			"mode":      "degraded",
		})
		return
	}

	queue := evergreen.GetEnvironment().RemoteQueue()

	taskIDs := map[string]struct{}{}
	tasks, err := host.FindStaleRunningTasks(heartbeatTimeoutThreshold, host.TaskHeartbeatPastCutoff)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	for _, t := range tasks {
		taskIDs[t.Id] = struct{}{}
	}
	j.logTasks(tasks, "heartbeat past cutoff, on running host")
	tasks, err = host.FindStaleRunningTasks(heartbeatTimeoutThreshold, host.TaskNoHeartbeatSinceDispatch)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	for _, t := range tasks {
		taskIDs[t.Id] = struct{}{}
	}
	j.logTasks(tasks, "no heartbeat since dispatch, on running host")
	tasks, err = host.FindStaleRunningTasks(heartbeatTimeoutThreshold, host.TaskUndispatchedHasHeartbeat)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	for _, t := range tasks {
		taskIDs[t.Id] = struct{}{}
	}
	j.logTasks(tasks, "undispatched task has a heartbeat, on running host")

	tasks, err = task.FindWithFields(task.ByStaleRunningTask(heartbeatTimeoutThreshold, task.HeartbeatPastCutoff), task.IdKey)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	for _, t := range tasks {
		taskIDs[t.Id] = struct{}{}
	}
	j.logTasks(tasks, "heartbeat past cutoff")
	tasks, err = task.FindWithFields(task.ByStaleRunningTask(heartbeatTimeoutThreshold, task.NoHeartbeatSinceDispatch), task.IdKey)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	for _, t := range tasks {
		taskIDs[t.Id] = struct{}{}
	}
	j.logTasks(tasks, "no heartbeat since dispatch")

	for id := range taskIDs {
		ts := utility.RoundPartOfHour(15)
		j.AddError(amboy.EnqueueUniqueJob(ctx, queue, NewTaskExecutionMonitorJob(id, ts.Format(TSFormat))))
	}
	grip.Info(message.Fields{
		"operation": "task-execution-timeout-populate",
		"num_tasks": len(tasks),
		"errors":    j.HasErrors(),
	})
}

func (j *taskExecutionTimeoutPopulationJob) logTasks(tasks []task.Task, reason string) {
	if len(tasks) == 0 {
		return
	}
	var taskIDs []string
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	grip.Info(message.Fields{
		"message":   "found stale tasks",
		"reason":    reason,
		"tasks":     taskIDs,
		"operation": j.Type().Name,
		"job_id":    j.ID(),
	})
}
