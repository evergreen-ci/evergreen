package units

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
)

const (
	taskStatsCollectorJobName  = "task-stats-collector"
	taskStatsCollectorInterval = time.Minute
)

func init() {
	registry.AddJobType(taskStatsCollectorJobName,
		func() amboy.Job { return makeTaskStatsCollector() })
}

type taskStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewTaskStatsCollector captures a single report of the status of
// tasks that have completed in the last minute.
func NewTaskStatsCollector(id string) amboy.Job {
	t := makeTaskStatsCollector()
	t.SetID(id)
	return t
}

func makeTaskStatsCollector() *taskStatsCollector {
	return &taskStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func (j *taskStatsCollector) Run() {
	defer j.MarkComplete()

	tasks, err := task.GetRecentTasks(taskStatsCollectorInterval)
	if err != nil {
		j.AddError(err)
		return
	}

	j.logger.Info(task.GetResultCounts(tasks))
}
