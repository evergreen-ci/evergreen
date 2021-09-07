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
	job.DistroId = distroId
	job.SetID(fmt.Sprintf("%s:%s:%s", checkBlockedTasks, distroId, ts))
	return job
}

func (j *checkBlockedTasksJob) Run(ctx context.Context) {
	queue, err := model.FindDistroTaskQueue(j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting task queue for distro '%s'", j.DistroId))
	}
	aliasQueue, err := model.FindDistroAliasTaskQueue(j.DistroId)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting alias task queue for distro '%s'", j.DistroId))
	}
	taskIds := []string{}
	lenQueue := len(queue.Queue)
	for _, item := range queue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
		}
	}

	lenAliasQueue := len(aliasQueue.Queue)
	for _, item := range aliasQueue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
		}
	}

	if len(taskIds) == 0 {
		grip.Debug(message.Fields{
			"message":         "no task IDs found for distro",
			"len_queue":       lenQueue,
			"len_alias_queue": lenAliasQueue,
			"distro":          j.DistroId,
			"job":             j.ID(),
			"source":          checkBlockedTasks,
		})
		return
	}

	tasksToCheck, err := task.Find(task.ByIds(taskIds))
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting tasks to check"))
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
	grip.Debug(message.Fields{
		"message":             "finished check blocked tasks job",
		"len_queue":           lenQueue,
		"len_alias_queue":     lenAliasQueue,
		"len_tasks":           len(tasksToCheck),
		"len_task_ids":        len(taskIds),
		"task_ids":            taskIds,
		"num_tasks_modified":  numTasksModified,
		"num_updating_checks": numChecksThatUpdatedTasks,
		"job":                 j.ID(),
		"source":              checkBlockedTasks,
		"distro":              j.DistroId,
	})
}

func checkUnmarkedBlockingTasks(t *task.Task, dependencyCaches map[string]task.Task) (int, error) {
	catcher := grip.NewBasicCatcher()

	dependenciesMet, err := t.DependenciesMet(dependencyCaches)
	if err != nil {
		return 0, errors.Wrapf(err, "error checking if dependencies met for task '%s'", t.Id)
	}
	if dependenciesMet {
		return 0, nil
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
		err = task.DeactivateDependencies(blockingDeactivatedTasks, evergreen.DefaultTaskActivator+".dispatcher")
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
		"exec_task":                          t.IsPartOfDisplay(),
		"source":                             checkBlockedTasks,
	})
	return numModified, catcher.Resolve()
}
