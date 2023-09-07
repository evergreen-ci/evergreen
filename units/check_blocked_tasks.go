package units

import (
	"context"
	"fmt"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"

	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
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
	queue, err := model.FindDistroTaskQueue(j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting task queue for distro '%s'", j.DistroId))
	}
	secondaryQueue, err := model.FindDistroSecondaryTaskQueue(j.DistroId)
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
		return
	}

	tasksToCheck, err := task.Find(task.ByIds(taskIds))
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting tasks to check in distro '%s'", j.DistroId))
		return
	}

	dependencyCache := map[string]task.Task{}
	numTasksModified := 0
	numChecksThatUpdatedTasks := 0
	for _, t := range tasksToCheck {
		numModified, err := checkUnmarkedBlockingTasks(&t, dependencyCache)
		j.AddError(err)
		numTasksModified += numModified
		if numTasksModified > 0 {
			numChecksThatUpdatedTasks++
		}
	}
}

func checkUnmarkedBlockingTasks(t *task.Task, dependencyCaches map[string]task.Task) (int, error) {
	catcher := grip.NewBasicCatcher()

	dependenciesMet, err := t.DependenciesMet(dependencyCaches)
	if err != nil {
		grip.Debug(message.Fields{
			"message":      "checking if dependencies met for task",
			"task_id":      t.Id,
			"activated_by": t.ActivatedBy,
			"depends_on":   t.DependsOn,
		})
		return 0, errors.Wrapf(err, "checking if dependencies met for task '%s'", t.Id)
	}
	if dependenciesMet {
		return 0, nil
	}

	blockingTasks, err := t.RefreshBlockedDependencies(dependencyCaches)
	catcher.Wrap(err, "getting blocking tasks")
	blockingTaskIds := []string{}
	if err == nil {
		for _, blockingTask := range blockingTasks {
			blockingTaskIds = append(blockingTaskIds, blockingTask.Id)
			err = model.UpdateBlockedDependencies(&blockingTask)
			catcher.Wrapf(err, "updating blocked dependencies for '%s'", blockingTask.Id)
		}
	}

	blockingDeactivatedTasks, err := t.BlockedOnDeactivatedDependency(dependencyCaches)
	catcher.Wrap(err, "getting blocked status")
	if err == nil && len(blockingDeactivatedTasks) > 0 {
		err = task.DeactivateDependencies(blockingDeactivatedTasks, evergreen.CheckBlockedTasksActivator)
		catcher.Add(err)
	}

	// also update the display task status in case it is out of date
	if t.IsPartOfDisplay() {
		catcher.Add(model.UpdateDisplayTaskForTask(t))
	}

	numModified := len(blockingTasks) + len(blockingDeactivatedTasks)
	grip.DebugWhen(numModified > 0, message.Fields{
		"message":                            "checked unmarked blocking tasks",
		"blocking_tasks_updated":             len(blockingTasks),
		"blocking_deactivated_tasks_updated": len(blockingDeactivatedTasks),
		"blocking_task_ids":                  blockingTaskIds,
		"exec_task":                          t.IsPartOfDisplay(),
		"source":                             checkBlockedTasks,
	})
	return numModified, catcher.Resolve()
}
