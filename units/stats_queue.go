package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const queueStatsCollectorJobName = "queue-stats-collector"

func init() {
	registry.AddJobType(queueStatsCollectorJobName,
		func() amboy.Job { return makeQueueStatsCollector() })
}

type queueStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func NewQueueStatsCollector(id string) amboy.Job {
	j := makeQueueStatsCollector()
	j.SetID(fmt.Sprintf("%s-%s", queueStatsCollectorJobName, id))
	return j
}

func makeQueueStatsCollector() *queueStatsCollector {
	j := &queueStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    queueStatsCollectorJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *queueStatsCollector) Run(_ context.Context) {
	defer j.MarkComplete()

	env := evergreen.GetEnvironment()
	if env == nil {
		j.AddError(errors.New("env is nil"))
		return
	}
	settings := env.Settings()

	finder := scheduler.GetTaskFinder(settings.Scheduler.TaskFinder)
	tasks, err := finder("")
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding runnable tasks"))
		return
	}
	grip.Info(message.Fields{
		"total_queue_length": len(tasks),
	})
}
