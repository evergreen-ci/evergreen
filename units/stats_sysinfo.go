package units

import (
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
)

const sysInfoStatsCollectorJobName = "sysinfo-stats-collector"

func init() {
	registry.AddJobType(sysInfoStatsCollectorJobName,
		func() amboy.Job { return makeSysInfoStatsCollector() })
}

type sysInfoStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewSysInfoStatsCollector reports basic system information and a
// report of the go runtime information, as provided by grip.
func NewSysInfoStatsCollector(id string) amboy.Job {
	j := makeSysInfoStatsCollector()
	j.SetID(id)
	return j
}

func makeSysInfoStatsCollector() *sysInfoStatsCollector {
	j := &sysInfoStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    sysInfoStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *sysInfoStatsCollector) Run() {
	defer j.MarkComplete()

	j.logger.Info(message.CollectSystemInfo())
	j.logger.Info(message.CollectGoStats())
}
