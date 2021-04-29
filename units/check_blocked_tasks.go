package units

import (
	"context"
	"fmt"
	"time"

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
	TaskIDs  []string `bson:"task_ids" json:"task_ids" yaml:"task_ids"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
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
func NewCheckBlockedTasksJob(distroId string, taskIDs []string) amboy.Job {
	job := makeCheckBlockedTasksJob()
	job.TaskIDs = taskIDs
	job.SetID(fmt.Sprintf("%s:%s:%s", checkBlockedTasks, distroId, time.Now().Round(time.Hour).String()))
	return job
}

func (j *checkBlockedTasksJob) Run(ctx context.Context) {
	tasksToCheck, err := task.Find(task.ByIds(j.TaskIDs))
	if err != nil {
		j.AddError(err)
		return
	}

	dependencyCache := map[string]task.Task{}
	for _, t := range tasksToCheck {
		j.AddError(model.CheckUnmarkedBlockingTasks(&t, dependencyCache))
	}

}
