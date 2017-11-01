package units

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type restartTasksJob struct {
	job.Base  `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger    grip.Journaler
	startTime time.Time
	endTime   time.Time
	user      string
	opts      model.RestartTaskOptions
}

// NewTasksRestartJob creates a job to restart failed tasks in a time range
func NewTasksRestartJob(startTime, endTime time.Time, user string, opts model.RestartTaskOptions) amboy.Job {
	job := restartTasksJob{
		logger:    logging.MakeGrip(grip.GetSender()),
		startTime: startTime,
		endTime:   endTime,
		user:      user,
		opts:      opts,
	}
	job.SetID(util.RandomString())
	return &job
}

func (j *restartTasksJob) Run() {
	results, err := model.RestartFailedTasks(j.startTime, j.endTime, j.user, j.opts)
	if err != nil {
		j.logger.Error(errors.Wrap(err, "error restarting failed tasks"))
		return
	}
	j.logger.Info(message.Fields{
		"message":         "tasks successfully restarted",
		"num":             len(results.TasksRestarted),
		"user":            j.user,
		"tasks_restarted": results.TasksRestarted,
		"tasks_errored":   results.TasksErrored,
	})
	j.MarkComplete()
}
