package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
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

	env  evergreen.Environment
	task *task.Task
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

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
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

	if j.task == nil {
		t, err := task.FindOneId(ctx, j.Task)
		if err != nil {
			j.AddRetryableError(errors.Wrapf(err, "finding task '%s'", j.Task))
			return
		}
		if t == nil {
			j.AddError(errors.Errorf("task '%s' not found", j.Task))
			return
		}
		j.task = t
	}

	msg := message.Fields{
		"job":                j.ID(),
		"job_type":           j.Type().Name,
		"task":               j.task.Id,
		"execution_platform": j.task.ExecutionPlatform,
		"host_id":            j.task.HostId,
	}

	// If the task has heartbeat since this job was queued, let it run.
	if j.task.LastHeartbeat.Add(evergreen.HeartbeatTimeoutThreshold).After(time.Now()) {
		msg["message"] = "refusing to clean up timed-out task because it has a recent heartbeat"
		grip.Info(msg)
		return
	}
	// If the task is already finished, don't try cleaning it up again.
	if j.task.IsFinished() {
		msg["message"] = "refusing to clean up timed-out task because it is already finished"
		grip.Info(msg)
		return
	}

	err = j.cleanUpTimedOutTask(ctx)
	if err != nil {
		msg["message"] = "failed to clean up timed-out task"
		grip.Warning(message.WrapError(err, msg))
		j.AddRetryableError(err)
		return
	}

	msg["message"] = "successfully cleaned up timed-out task"
	grip.Info(msg)
}

// cleanUpTimedOutTask cleans up a single stale task that has exceeded the task
// heartbeat timeout.
func (j *taskExecutionTimeoutJob) cleanUpTimedOutTask(ctx context.Context) error {
	host, err := host.FindOne(ctx, host.ById(j.task.HostId))
	if err != nil {
		return errors.Wrapf(err, "finding host '%s' for task '%s'", j.task.HostId, j.task.Id)
	}

	// if there's no relevant host and the task is not a display task, something went wrong
	if host == nil {
		grip.ErrorWhen(!j.task.DisplayOnly, message.Fields{
			"message":   "no entry found for host",
			"task":      j.task.Id,
			"host_id":   j.task.HostId,
			"operation": "cleanup timed out task",
			"job":       j.ID(),
		})
		return errors.WithStack(j.task.MarkUnscheduled(ctx))
	}

	if host.RunningTask == j.task.Id {
		// Check if the host was externally terminated before clearing the
		// host's running task. When the running task is cleared on the host, an
		// agent or agent monitor deploy might run, which updates the LCT and
		// prevents detection of external termination until the deploy job runs
		// out of retries.
		terminated, err := handleExternallyTerminatedHost(ctx, j.ID(), j.env, host)
		if err != nil {
			return errors.Wrapf(err, "checking host '%s 'with timed out task '%s' for external termination", host.Id, j.task.Id)
		}
		if terminated {
			// If the host has been externally terminated, then this is treated
			// as a task failure due to stranding on a dead host rather than a
			// stale heartbeat. The host termination process will deal with
			// fixing the stranded task.
			return nil
		}
		if err = host.ClearRunningAndSetLastTask(ctx, j.task); err != nil {
			return errors.Wrapf(err, "clearing running task '%s' from host '%s'", j.task.Id, host.Id)
		}
	}

	if err := model.FixStaleTask(ctx, j.env.Settings(), j.task); err != nil {
		return errors.Wrapf(err, "resetting stale task '%s'", j.task.Id)
	}

	return nil
}

////////////////////////////////////////////////////////////////////////
//
// Population Job

type taskExecutionTimeoutPopulationJob struct {
	env      evergreen.Environment
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

// NewTaskExecutionMonitorPopulateJob returns a job to populate the queue with
// jobs to check for stale tasks.
func NewTaskExecutionMonitorPopulateJob(id string) amboy.Job {
	j := makeTaskExecutionTimeoutPopulateJob()
	j.SetID(fmt.Sprintf("%s.%s", j.Type().Name, id))
	return j
}

func (j *taskExecutionTimeoutPopulationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
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

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	queue := j.env.RemoteQueue()

	tasks, err := task.FindWithFields(ctx, task.ByStaleRunningTask(evergreen.HeartbeatTimeoutThreshold), task.IdKey)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding tasks with timed-out or stale heartbeats"))
		return
	}
	var taskIDs []string
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.Id)
	}
	grip.InfoWhen(len(taskIDs) > 0, message.Fields{
		"message":   "found stale tasks",
		"tasks":     taskIDs,
		"operation": j.Type().Name,
		"job_id":    j.ID(),
	})

	for _, id := range taskIDs {
		ts := utility.RoundPartOfHour(15)
		j.AddError(amboy.EnqueueUniqueJob(ctx, queue, NewTaskExecutionMonitorJob(id, ts.Format(TSFormat))))
	}
	grip.Info(message.Fields{
		"operation": "task-execution-timeout-populate",
		"num_tasks": len(tasks),
		"errors":    j.HasErrors(),
	})
}
