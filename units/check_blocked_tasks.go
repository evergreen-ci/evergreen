package units

import (
	"context"
	"fmt"
	"time"

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
	for _, item := range queue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
		}
	}
	for _, item := range aliasQueue.Queue {
		if !item.IsDispatched && len(item.Dependencies) > 0 {
			taskIds = append(taskIds, item.Id)
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
		j.AddError(model.CheckUnmarkedBlockingTasks(&t, dependencyCache))
	}
}
