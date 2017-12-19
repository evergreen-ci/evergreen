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
		func() amboy.Job { return makeLatencyStatsCollector() })
}

type latencyStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewLatencyStatsCollector captures a single report of the latency of
// tasks that have started in the last minute.
func NewLatencyStatsCollector(id string) amboy.Job {
	t := makeTaskStatsCollector()
	t.SetID(id)
	return t
}

func makeLatencyStatsCollector() *latencyStatsCollector {
	return &latencyStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    latencyStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func (j *latencyStatsCollector) Run() {
	defer j.MarkComplete()

	latencies, err := model.AverageTaskLatency(time.Minute)
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding task latencies"))
		return
	}
	j.logger.Info(latencies)
}
