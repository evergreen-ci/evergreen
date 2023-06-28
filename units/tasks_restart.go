package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const restartTasksJobName = "restart-tasks"

func init() {
	registry.AddJobType(restartTasksJobName, func() amboy.Job { return makeTaskRestartJob() })
}

type restartTasksJob struct {
	Opts     model.RestartOptions `bson:"restart_options" json:"restart_options"`
	job.Base `bson:"job_base" json:"job_base"`
}

func makeTaskRestartJob() *restartTasksJob {
	j := &restartTasksJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    restartTasksJobName,
				Version: 1,
			},
		},
	}
	return j
}

// NewTasksRestartJob creates a job to restart failed tasks in a time range
func NewTasksRestartJob(opts model.RestartOptions) amboy.Job {
	job := makeTaskRestartJob()
	job.Opts = opts
	job.SetID(fmt.Sprintf("restart-tasks-%d-%d", opts.StartTime.Unix(), opts.EndTime.Unix()))
	job.SetPriority(1)
	return job
}

func (j *restartTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	results, err := model.RestartFailedTasks(ctx, j.Opts)
	if err != nil {
		j.AddError(errors.Wrap(err, "restarting failed tasks"))
		return
	}

	grip.Info(message.Fields{
		"message":         "tasks successfully restarted",
		"num":             len(results.ItemsRestarted),
		"tasks_restarted": results.ItemsRestarted,
		"tasks_errored":   results.ItemsErrored,
		"user":            j.Opts.User,
		"start_at":        j.Opts.StartTime,
		"end_at":          j.Opts.EndTime,
	})
}
