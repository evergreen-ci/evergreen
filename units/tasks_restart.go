package units

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const restartTasksJobName = "restart-tasks"

func init() {
	registry.AddJobType(restartTasksJobName, func() amboy.Job { return makeTaskRestartJob() })
}

type restartTasksJob struct {
	Opts     model.RestartTaskOptions `bson:"restart_options" json:"restart_options" yaml:"restart_options"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	logger grip.Journaler
}

func makeTaskRestartJob() *restartTasksJob {
	j := &restartTasksJob{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    restartTasksJobName,
				Version: 1,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewTasksRestartJob creates a job to restart failed tasks in a time range
func NewTasksRestartJob(opts model.RestartTaskOptions) amboy.Job {
	job := makeTaskRestartJob()
	job.Opts = opts
	job.SetID(fmt.Sprintf("restart-tasks-%d-%d", opts.StartTime.Unix(), opts.EndTime.Unix()))
	return job
}

func (j *restartTasksJob) Run() {
	defer j.MarkComplete()
	results, err := model.RestartFailedTasks(j.Opts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error restarting failed tasks"))
		return
	}

	j.logger.Info(message.Fields{
		"message":         "tasks successfully restarted",
		"num":             len(results.TasksRestarted),
		"tasks_restarted": results.TasksRestarted,
		"tasks_errored":   results.TasksErrored,
		"user":            j.Opts.User,
		"start_at":        j.Opts.StartTime,
		"end_at":          j.Opts.EndTime,
	})
}
