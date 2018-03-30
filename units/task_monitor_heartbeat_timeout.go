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
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	heartbeatTimeoutThreshold   = 7 * time.Minute
	taskHeartbeatTimeoutJobName = "task-timeout-heartbeat"
)

func init() {
	registry.AddJobType(taskHeartbeatTimeoutJobName, func() amboy.Job {
		return makeTaskMonitorTimedoutHeartbeats()
	})
}

type taskHeartbeatTimeoutJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeTaskMonitorTimedoutHeartbeats() *taskHeartbeatTimeoutJob {
	j := &taskHeartbeatTimeoutJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskHeartbeatTimeoutJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewTaskHeartbeatMonitorJob(id string) amboy.Job {
	j := makeTaskMonitorTimedoutHeartbeats()
	j.SetID(fmt.Sprintf("%s.%s", taskHeartbeatTimeoutJobName, id))
	return j
}

func (j *taskHeartbeatTimeoutJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "monitor is disabled",
			"impact":  "skipping task heartbeat cleanup job",
			"mode":    "degraded",
		})
		return
	}

	threshold := time.Now().Add(-heartbeatTimeoutThreshold)

	tasks, err := task.Find(task.ByRunningLastHeartbeat(threshold))
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding tasks with timed-out heartbeats"))
		return
	}

	for _, task := range tasks {
		msg := message.Fields{
			"operation": j.Type().Name,
			"runner":    "monitor",
			"id":        j.ID(),
			"task":      task.Id,
			"host":      task.HostId,
		}

		if err := cleanUpTimedOutTask(task); err != nil {
			grip.Warning(message.WrapError(err, msg))
			j.AddError(err)
			continue
		}

		grip.Debug(msg)
	}

	grip.Info(message.Fields{
		"operation": j.Type().Name,
		"runner":    "monitor",
		"id":        j.ID(),
		"num_tasks": len(tasks),
	})
}

// function to clean up a single task
func cleanUpTimedOutTask(t task.Task) error {
	// get the host for the task
	host, err := host.FindOne(host.ById(t.HostId))
	if err != nil {
		return errors.Wrapf(err, "error finding host %s for task %s",
			t.HostId, t.Id)
	}

	// if there's no relevant host, something went wrong
	if host == nil {
		grip.Error(message.Fields{
			"runner":  "monitor",
			"message": "no entry found for host",
			"host":    t.HostId,
		})
		return errors.WithStack(t.MarkUnscheduled())
	}

	// if the host still has the task as its running task, clear it.
	if host.RunningTask == t.Id {
		// clear out the host's running task
		if err = host.ClearRunningTask(t.Id, time.Now()); err != nil {
			return errors.Wrapf(err, "error clearing running task %v from host %v: %v",
				t.Id, host.Id)
		}
	}

	detail := &apimodels.TaskEndDetail{
		Description: task.AgentHeartbeat,
		TimedOut:    true,
		Status:      evergreen.TaskFailed,
	}

	// try to reset the task
	taskID := t.Id
	if t.IsPartOfDisplay() {
		taskID = t.DisplayTask.Id
	}
	if err := model.TryResetTask(taskID, "", "monitor", detail); err != nil {
		return errors.Wrapf(err, "error trying to reset task %s", t.Id)
	}

	return nil
}
