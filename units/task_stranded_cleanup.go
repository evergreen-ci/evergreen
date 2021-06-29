package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewStrandedTaskCleanupJob(id string) amboy.Job {
	j := makeStrandedTaskCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", taskStrandedCleanupJobName, id))
	return j
}

func (j *taskStrandedCleanupJob) Run(ctx context.Context) {
	hosts, err := host.FindTerminatedHostsRunningTasks()
	if err != nil {
		j.AddError(err)
		return
	}

	if len(hosts) == 0 {
		return
	}

	taskIDs := []string{}
	hostIDs := []string{}

	for _, h := range hosts {
		if h.RunningTask == "" {
			continue
		}

		taskIDs = append(taskIDs, h.RunningTask)
		hostIDs = append(hostIDs, h.Id)

		j.AddError(model.ClearAndResetStrandedTask(&h))
	}

	tasks, err := task.FindStuckDispatching()
	if err != nil {
		j.AddError(err)
	}

	tasksToDeactivate := []task.Task{}
	for _, t := range tasks {
		if time.Since(t.CreateTime) >= 2*7*24*time.Hour {
			tasksToDeactivate = append(tasksToDeactivate, t)
		} else {
			j.AddError(model.TryResetTask(t.Id, evergreen.User, j.ID(), &t.Details))
		}
	}
	if len(tasksToDeactivate) > 0 {
		err = task.DeactivateTasks(tasksToDeactivate, j.ID())
		j.AddError(err)
	}

	grip.InfoWhen(!j.HasErrors(),
		message.Fields{
			"job":       j.ID(),
			"op":        j.Type().Name,
			"num_hosts": len(hosts),
			"num_tasks": len(taskIDs),
			"tasks":     taskIDs,
			"hosts":     hostIDs,
		})
}
