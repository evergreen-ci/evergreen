package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const queueStatsCollectorJobName = "queue-stats-collector"

func init() {
	registry.AddJobType(queueStatsCollectorJobName, func() amboy.Job {
		return makeQueueStatsCollector()
	})
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
	return j
}

func (j *queueStatsCollector) Run(_ context.Context) {
	defer j.MarkComplete()

	env := evergreen.GetEnvironment()
	if env == nil {
		j.AddError(errors.New("env is nil"))
		return
	}

	queues, err := model.FindAllTaskQueues()
	if err != nil {
		j.AddError(err)
		return
	}

	var total int
	var totalDur time.Duration

	for _, q := range queues {
		total += q.DistroQueueInfo.Length
		totalDur += q.DistroQueueInfo.ExpectedDuration
	}

	grip.Info(message.Fields{
		"total_queue_length": total,
		"total_dur_secs":     totalDur.Seconds(),
		"total_dur_mins":     totalDur.Minutes(),
		"total_dur_hours":    totalDur.Hours(),
		"distros":            len(queues),
	})
}
