package units

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
)

const hostStatsCollectorJobName = "host-stats-collector"

func init() {
	registry.AddJobType(hostStatsCollectorJobName,
		func() amboy.Job { return makeHostStatsCollector() })
}

type hostStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewHostStatsCollector logs statistics about host utilization per
// distro to the default grip logger.
func NewHostStatsCollector(id string) amboy.Job {
	j := makeHostStatsCollector()
	j.SetID(id)
	return j
}

func makeHostStatsCollector() *hostStatsCollector {
	return &hostStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func (j *hostStatsCollector) Run() {
	defer j.MarkComplete()

	hosts, err := host.GetStatsByDistro()
	if err != nil {
		j.AddError(err)
		return
	}

	j.logger.Info(message.Fields{
		"report": "host stats by distro",
		"data":   hosts,
	})
}
