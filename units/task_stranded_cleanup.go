package units

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const taskStrandedCleanupJobName = "task-stranded-cleanup"

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
	j.SetID("%s.%s", taskStrandedCleanupJobName, id)
	return j
}

func (j *taskStrandedCleanupJob) Run(ctx context.Context) {
	hosts, err := host.FindTerminatedHostsRunningTasks()
	if err != nil {
		j.AddError(err)
		return
	}

	seenTasks := []string{}

	for _, h := range hosts {
		seenTasks = append(seenTasks, h.RunningTask)

		var t *task.Task

		t, err = task.FindOne(task.ById(h.RunningTask))
		if err != nil {
			j.AddError(err)
		}

		if err = h.ClearRunningTask(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"job_type": j.Type().Name,
				"message":  "Error clearing running task for host",
				"provider": h.Distro.Provider,
				"host":     h.Id,
				"job":      j.ID(),
				"task":     t.Id,
			}))
		}

		if !t.IsFinished() {
			j.AddError(model.TryResetTask(t.Id, "mci", evergreen.MonitorPackage, &apimodels.TaskEndDetail{
				Status: evergreen.TaskFailed,
				Type:   "system",
			}))
		}
	}

	grip.Info(message.Fields{
		"job":       j.ID(),
		"op":        j.Type().Name,
		"num_hosts": len(hosts),
		"num_tasks": len(seenTasks),
		"tasks":     seenTasks,
	})
}
