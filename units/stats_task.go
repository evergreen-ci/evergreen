package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
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
	t.SetID(fmt.Sprintf("%s-%s", taskStatsCollectorJobName, id))
	return t
}

func makeTaskStatsCollector() *taskStatsCollector {
	j := &taskStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStatsCollectorJobName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *taskStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.BackgroundStatsDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	tasks, err := task.GetRecentTasks(taskStatsCollectorInterval)
	if err != nil {
		j.AddError(err)
		return
	}

	j.logger.Info(task.GetResultCounts(tasks))
}
