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
		return makeStrandedTaskCleanupJob(nil)
	})
}

type taskStrandedCleanupJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env evergreen.Environment
}

func makeStrandedTaskCleanupJob(env evergreen.Environment) *taskStrandedCleanupJob {
	j := &taskStrandedCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStrandedCleanupJobName,
				Version: 0,
			},
		},
		env: env,
	}
	return j
}

// NewStrandedTaskCleanupJob returns a job to detect and clean up tasks that:
// - Have been stranded on hosts that are already terminated.
// - Have stuck dispatching for too long.
// - Are virtual tasks stranded by a runner that finished without push-completing them.
func NewStrandedTaskCleanupJob(env evergreen.Environment, id string) amboy.Job {
	j := makeStrandedTaskCleanupJob(env)
	j.SetID(fmt.Sprintf("%s.%s", taskStrandedCleanupJobName, id))
	return j
}

func (j *taskStrandedCleanupJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	j.AddError(errors.Wrap(j.fixTasksStrandedOnTerminatedHosts(ctx), "fixing tasks stranded on already-terminated hosts"))
	j.AddError(errors.Wrap(j.fixTasksStuckDispatching(ctx), "fixing tasks that are stuck dispatching"))
	j.AddError(errors.Wrap(j.fixStrandedVirtualTasks(ctx), "fixing stranded virtual tasks"))
}

func (j *taskStrandedCleanupJob) fixTasksStrandedOnTerminatedHosts(ctx context.Context) error {
	hosts, err := host.FindTerminatedHostsRunningTasks(ctx)
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

		catcher.Wrapf(model.ClearAndResetStrandedHostTask(ctx, j.env.Settings(), &h, "host was terminated while running the task"), "fixing stranded host task '%s' execution '%d' on host '%s'", h.RunningTask, h.RunningTaskExecution, h.Id)
	}

	grip.Info(ctx, message.Fields{
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
	tasks, err := task.FindStuckDispatching(ctx)
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
				Type:                evergreen.CommandTypeSystem,
				Status:              evergreen.TaskFailed,
				Description:         evergreen.TaskDescriptionStranded,
				SystemFailureReason: "task was dispatched to a host but never started running",
			}
			catcher.Wrapf(model.TryResetTask(ctx, j.env.Settings(), t.Id, evergreen.User, j.ID(), details), "resetting task '%s'", t.Id)
			tasksReset = append(tasksReset, t)
		}
	}
	if len(tasksToDeactivate) > 0 {
		err = task.DeactivateTasks(ctx, tasksToDeactivate, true, j.ID(), "")
		catcher.Wrapf(err, "deactivating tasks exceeding the unschedulable threshold")

		grip.Info(ctx, message.Fields{
			"message":   "deactivated tasks that are stuck dispatching and have exceeded the scheduling threshold",
			"job":       j.ID(),
			"op":        j.Type().Name,
			"num_tasks": len(tasksToDeactivate),
			"tasks":     tasksToDeactivate,
		})
	}

	grip.InfoWhen(ctx, len(tasksReset) > 0, message.Fields{
		"message":   "reset tasks that are stuck dispatching",
		"job":       j.ID(),
		"op":        j.Type().Name,
		"num_tasks": len(tasksReset),
		"tasks":     tasksReset,
	})

	return catcher.Resolve()
}

func (j *taskStrandedCleanupJob) fixStrandedVirtualTasks(ctx context.Context) error {
	tasks, err := task.FindStrandedVirtualTasks(ctx)
	if err != nil {
		return errors.Wrap(err, "finding stranded virtual tasks")
	}

	catcher := grip.NewBasicCatcher()
	var fixed []string
	for i := range tasks {
		t := tasks[i]
		if err := model.FixStrandedVirtualTask(ctx, j.env.Settings(), &t); err != nil {
			catcher.Wrapf(err, "fixing stranded virtual task '%s'", t.Id)
			continue
		}
		fixed = append(fixed, t.Id)
	}

	grip.InfoWhen(ctx, len(fixed) > 0, message.Fields{
		"message":   "fixed stranded virtual tasks",
		"job":       j.ID(),
		"op":        j.Type().Name,
		"num_tasks": len(fixed),
		"tasks":     fixed,
	})

	return catcher.Resolve()
}
