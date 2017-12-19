package units

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/pkg/errors"
)

const (
	latencyStatsCollectorJobName  = "latency-stats-collector"
	latencyStatsCollectorInterval = time.Minute
)

func init() {
	registry.AddJobType(latencyStatsCollectorJobName,
		func() amboy.Job { return makeLatencyStatsCollector(time.Minute) })
}

type latencyStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	duration time.Duration
}

// NewLatencyStatsCollector captures a single report of the latency of
// tasks that have started in the last minute.
func NewLatencyStatsCollector(id string, duration time.Duration) amboy.Job {
	t := makeLatencyStatsCollector(duration)
	t.SetID(id)
	return t
}

func makeLatencyStatsCollector(duration time.Duration) *latencyStatsCollector {
	return &latencyStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    latencyStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
		duration: duration,
	}
}

func (j *latencyStatsCollector) Run() {
	defer j.MarkComplete()
	logger := logging.MakeGrip(grip.GetSender())

	latencies, err := model.AverageTaskLatency(j.duration)
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding task latencies"))
		return
	}
	logger.Info(latencies)
}
