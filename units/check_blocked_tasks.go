package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	checkBlockedTasks = "check_blocked_tasks"
)

func init() {
	registry.AddJobType(checkBlockedTasks, func() amboy.Job { return makeCheckBlockedTasksJob() })
}

type checkBlockedTasksJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	DistroId string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
}

func makeCheckBlockedTasksJob() *checkBlockedTasksJob {
	j := &checkBlockedTasksJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    checkBlockedTasks,
				Version: 0,
			},
		},
	}
	return j
}

// NewCheckBlockedTasksJob creates a job to audit the dependency state for tasks
// in the task queues. If it finds any mismatches in dependency state, it fixes
// them.
func NewCheckBlockedTasksJob(distroId string, ts time.Time) amboy.Job {
	job := makeCheckBlockedTasksJob()
	job.DistroId = distroId
	job.SetID(fmt.Sprintf("%s:%s:%s", checkBlockedTasks, distroId, ts))
	return job
}

func (j *checkBlockedTasksJob) Run(ctx context.Context) {
	var tasksToCheck []task.Task
	if j.DistroId != "" {
		tasksToCheck = j.getDistroTasksToCheck(ctx)
	}
	dependencyCache := map[string]task.Task{}
	for _, t := range tasksToCheck {
		j.AddError(errors.Wrapf(checkUnmarkedBlockingTasks(ctx, &t, dependencyCache), "checking task '%s'", t.Id))
	}
}

func (j *checkBlockedTasksJob) getDistroTasksToCheck(ctx context.Context) []task.Task {
	queue, err := model.FindDistroTaskQueue(ctx, j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting task queue for distro '%s'", j.DistroId))
	}
	secondaryQueue, err := model.FindDistroSecondaryTaskQueue(ctx, j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting alias task queue for distro '%s'", j.DistroId))
	}
	taskIds := []string{}
	for _, item := range queue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
		}
	}

	for _, item := range secondaryQueue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
		}
	}

	if len(taskIds) == 0 {
		grip.Debug(message.Fields{
			"message":             "no task IDs found for distro",
			"len_queue":           len(queue.Queue),
			"len_secondary_queue": len(secondaryQueue.Queue),
			"distro":              j.DistroId,
			"job":                 j.ID(),
			"source":              checkBlockedTasks,
		})
		return nil
	}

	tasksToCheck, err := task.Find(ctx, task.PotentiallyBlockedTasksByIds(taskIds))
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting tasks to check in distro '%s'", j.DistroId))
		return nil
	}
	return tasksToCheck
}

// checkUnmarkedBlockingTasks checks if the task is blocked by any of its dependencies.
// If it is blocked, it gets the blocking tasks and updates their dependencies.
// For blocking tasks that are finished/blocked, it updates their blocked status.
// For blocking tasks that are deactivated and not finished, it deactivates their dependencies.
func checkUnmarkedBlockingTasks(ctx context.Context, t *task.Task, dependencyCaches map[string]task.Task) error {
	catcher := grip.NewBasicCatcher()

	dependenciesMet, err := t.DependenciesMet(ctx, dependencyCaches)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":      "could not check if dependencies met for task",
			"task_id":      t.Id,
			"activated_by": t.ActivatedBy,
			"depends_on":   t.DependsOn,
		}))
		return errors.Wrapf(err, "checking if dependencies met for task '%s'", t.Id)
	}
	if dependenciesMet {
		return nil
	}

	finishedBlockingTasks, err := t.GetFinishedBlockingDependencies(ctx, dependencyCaches)
	catcher.Wrap(err, "getting blocking tasks")
	blockingTaskIds := []string{}
	if err == nil {
		for _, blockingTask := range finishedBlockingTasks {
			blockingTaskIds = append(blockingTaskIds, blockingTask.Id)
			err = model.UpdateBlockedDependencies(ctx, []task.Task{blockingTask}, false)
			catcher.Wrapf(err, "updating blocked dependencies for '%s'", blockingTask.Id)
			if err != nil {
				err = t.MarkDependenciesFinished(ctx, false)
				catcher.Wrapf(err, "updating dependencies as not finished for '%s'", blockingTask.Id)
			}
		}
	}

	deactivatedBlockingTasks, err := t.GetDeactivatedBlockingDependencies(ctx, dependencyCaches)
	catcher.Wrap(err, "getting blocked status")
	if err == nil && len(deactivatedBlockingTasks) > 0 {
		err = task.DeactivateDependencies(ctx, deactivatedBlockingTasks, evergreen.CheckBlockedTasksActivator)
		catcher.Add(err)
	}

	// also update the display task status in case it is out of date
	if t.IsPartOfDisplay(ctx) {
		catcher.Add(model.UpdateDisplayTaskForTask(ctx, t))
	}

	numModified := len(finishedBlockingTasks) + len(deactivatedBlockingTasks)
	grip.DebugWhen(numModified > 0, message.Fields{
		"message":                            "checked unmarked blocking tasks",
		"blocking_finished_tasks_updated":    len(finishedBlockingTasks),
		"blocking_deactivated_tasks_updated": len(deactivatedBlockingTasks),
		"blocking_task_ids":                  blockingTaskIds,
		"exec_task":                          t.IsPartOfDisplay(ctx),
		"source":                             checkBlockedTasks,
	})
	return catcher.Resolve()
}
