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
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const taskStrandedCleanupJobName = "task-stranded-cleanup"

func init() {
	registry.AddJobType(taskStrandedCleanupJobName, func() amboy.Job {
		return makeStrandedTaskCleanupJob()
	})
}

type taskStrandedCleanupJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeStrandedTaskCleanupJob() *taskStrandedCleanupJob {
	j := &taskStrandedCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStrandedCleanupJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewStrandedTaskCleanupJob returns a job to detect and clean up tasks that:
// - Have been stranded on hosts that are already terminated.
// - Have stuck dispatching for too long.
func NewStrandedTaskCleanupJob(id string) amboy.Job {
	j := makeStrandedTaskCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", taskStrandedCleanupJobName, id))
	return j
}

func (j *taskStrandedCleanupJob) Run(ctx context.Context) {
	j.AddError(errors.Wrap(j.fixTasksStrandedOnTerminatedHosts(ctx), "fixing tasks stranded on already-terminated hosts"))
	j.AddError(errors.Wrap(j.fixTasksStuckDispatching(ctx), "fixing tasks that are stuck dispatching"))
}

func (j *taskStrandedCleanupJob) fixTasksStrandedOnTerminatedHosts(ctx context.Context) error {
	hosts, err := host.FindTerminatedHostsRunningTasks()
	if err != nil {
		return errors.Wrap(err, "finding already-terminated hosts running tasks")
	}

	if len(hosts) == 0 {
		return nil
	}

	var taskIDs []string
	var hostIDs []string

	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		taskIDs = append(taskIDs, h.RunningTask)
		hostIDs = append(hostIDs, h.Id)

		catcher.Wrapf(model.ClearAndResetStrandedHostTask(ctx, evergreen.GetEnvironment().Settings(), &h), "fixing stranded host task '%s' execution '%d' on host '%s'", h.RunningTask, h.RunningTaskExecution, h.Id)
	}

	grip.Info(message.Fields{
		"message":   "fixed tasks stranded on already-terminated hosts",
		"job":       j.ID(),
		"op":        j.Type().Name,
		"num_hosts": len(hosts),
		"num_tasks": len(taskIDs),
		"tasks":     taskIDs,
		"hosts":     hostIDs,
	})

	return catcher.Resolve()
}

func (j *taskStrandedCleanupJob) fixTasksStuckDispatching(ctx context.Context) error {
	tasks, err := task.FindStuckDispatching()
	if err != nil {
		return errors.Wrap(err, "finding tasks that are stuck dispatching")
	}

	catcher := grip.NewBasicCatcher()
	var tasksToDeactivate []task.Task
	var tasksReset []task.Task
	for _, t := range tasks {
		if time.Since(t.ActivatedTime) >= task.UnschedulableThreshold {
			tasksToDeactivate = append(tasksToDeactivate, t)
		} else {
			details := &apimodels.TaskEndDetail{
				Type:        evergreen.CommandTypeSystem,
				Status:      evergreen.TaskFailed,
				Description: evergreen.TaskDescriptionStranded,
			}
			catcher.Wrapf(model.TryResetTask(ctx, evergreen.GetEnvironment().Settings(), t.Id, evergreen.User, j.ID(), details), "resetting task '%s'", t.Id)
			tasksReset = append(tasksReset, t)
		}
	}
	if len(tasksToDeactivate) > 0 {
		err = task.DeactivateTasks(tasksToDeactivate, true, j.ID())
		catcher.Wrapf(err, "deactivating tasks exceeding the unschedulable threshold")

		grip.Info(message.Fields{
			"message":   "deactivated tasks that are stuck dispatching and have exceeded the scheduling threshold",
			"job":       j.ID(),
			"op":        j.Type().Name,
			"num_tasks": len(tasksToDeactivate),
			"tasks":     tasksToDeactivate,
		})
	}

	grip.InfoWhen(len(tasksReset) > 0, message.Fields{
		"message":   "reset tasks that are stuck dispatching",
		"job":       j.ID(),
		"op":        j.Type().Name,
		"num_tasks": len(tasksReset),
		"tasks":     tasksReset,
	})

	return catcher.Resolve()
}
