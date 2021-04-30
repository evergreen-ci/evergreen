package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"

	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
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
	env      evergreen.Environment
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
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewCheckBlockedTasksJob creates a job to run repotracker against a repository.
// The code creating this job is responsible for verifying that the project
// should track push events. We want to limit this job to once an hour for each distro.
func NewCheckBlockedTasksJob(distroId string, ts time.Time) amboy.Job {
	job := makeCheckBlockedTasksJob()
	job.SetID(fmt.Sprintf("%s:%s:%s", checkBlockedTasks, distroId, ts))
	return job
}

func (j *checkBlockedTasksJob) Run(ctx context.Context) {
	queue, err := model.LoadTaskQueue(j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting task queue for distro '%s'", j.DistroId))
	}
	aliasQueue, err := model.LoadDistroAliasTaskQueue(j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting alias task queue for distro '%s'", j.DistroId))
	}
	taskIds := []string{}
	if queue != nil {
		for _, item := range queue.Queue {
			if !item.IsDispatched && len(item.Dependencies) > 0 {
				taskIds = append(taskIds, item.Id)
			}
		}
	}

	if aliasQueue != nil {
		for _, item := range aliasQueue.Queue {
			if !item.IsDispatched && len(item.Dependencies) > 0 {
				taskIds = append(taskIds, item.Id)
			}
		}
	}

	if len(taskIds) == 0 {
		return
	}

	tasksToCheck, err := task.Find(task.ByIds(taskIds))
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting tasks to check"))
		return
	}

	dependencyCache := map[string]task.Task{}
	for _, t := range tasksToCheck {
		j.AddError(checkUnmarkedBlockingTasks(&t, dependencyCache))
	}
}

func checkUnmarkedBlockingTasks(t *task.Task, dependencyCaches map[string]task.Task) error {
	catcher := grip.NewBasicCatcher()

	dependenciesMet, err := t.DependenciesMet(dependencyCaches)
	if err != nil {
		return errors.Wrapf(err, "error checking if dependencies met for task '%s'", t.Id)
	}
	if dependenciesMet {
		return nil
	}

	blockingTasks, err := t.RefreshBlockedDependencies(dependencyCaches)
	catcher.Add(errors.Wrap(err, "can't get blocking tasks"))
	if err == nil {
		for _, task := range blockingTasks {
			catcher.Add(errors.Wrapf(model.UpdateBlockedDependencies(&task), "can't update blocked dependencies for '%s'", task.Id))
		}
	}

	blockingDeactivatedTasks, err := t.BlockedOnDeactivatedDependency(dependencyCaches)
	catcher.Add(errors.Wrap(err, "can't get blocked status"))
	if err == nil && len(blockingDeactivatedTasks) > 0 {
		var deactivatedDependencies []task.Task
		deactivatedDependencies, err = task.DeactivateDependencies(blockingDeactivatedTasks, evergreen.DefaultTaskActivator+".dispatcher")
		catcher.Add(err)
		if err == nil {
			catcher.Add(build.SetManyCachedTasksActivated(deactivatedDependencies, false))
		}
	}

	// also update the display task status in case it is out of date
	if t.IsPartOfDisplay() {
		parent, err := t.GetDisplayTask()
		catcher.Add(err)
		if parent != nil {
			catcher.Add(model.UpdateDisplayTask(parent))
		}
	}

	grip.DebugWhen(len(blockingTasks)+len(blockingDeactivatedTasks) > 0, message.Fields{
		"message":                            "checked unmarked blocking tasks",
		"blocking_tasks_updated":             len(blockingTasks),
		"blocking_deactivated_tasks_updated": len(blockingDeactivatedTasks),
		"exec_task":                          t.IsPartOfDisplay(),
		"source":                             checkBlockedTasks,
	})
	return catcher.Resolve()
}
