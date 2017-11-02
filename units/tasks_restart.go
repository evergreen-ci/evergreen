package units

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const restartTasksJobName = "restart-tasks"

func init() {
	registry.AddJobType(restartTasksJobName,
		func() amboy.Job { return &restartTasksJob{} })
}

type restartTasksJob struct {
	job.Base  `bson:"job_base" json:"job_base" yaml:"job_base"`
	StartTime time.Time                `bson:"start_time" json:"start_time" yaml:"start_time"`
	EndTime   time.Time                `bson:"end_time" json:"end_time" yaml:"end_time"`
	User      string                   `bson:"user" json:"user" yaml:"user"`
	Opts      model.RestartTaskOptions `bson:"restart_options" json:"restart_options" yaml:"restart_options"`

	logger grip.Journaler
}

// NewTasksRestartJob creates a job to restart failed tasks in a time range
func NewTasksRestartJob(startTime, endTime time.Time, user string, opts model.RestartTaskOptions) amboy.Job {
	job := restartTasksJob{
		logger:    logging.MakeGrip(grip.GetSender()),
		StartTime: startTime,
		EndTime:   endTime,
		User:      user,
		Opts:      opts,
	}
	job.JobType = amboy.JobType{
		Name:    restartTasksJobName,
		Version: 0,
		Format:  amboy.BSON,
	}
	job.SetID(util.RandomString())
	return &job
}

func (j *restartTasksJob) Run() {
	defer j.MarkComplete()
	results, err := model.RestartFailedTasks(j.StartTime, j.EndTime, j.User, j.Opts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error restarting failed tasks"))
		return
	}
	j.logger.Info(message.Fields{
		"message":         "tasks successfully restarted",
		"num":             len(results.TasksRestarted),
		"user":            j.User,
		"tasks_restarted": results.TasksRestarted,
		"tasks_errored":   results.TasksErrored,
	})
}
