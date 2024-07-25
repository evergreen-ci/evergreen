package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostStatsCollectorJobName = "host-stats-collector"

func init() {
	registry.AddJobType(hostStatsCollectorJobName, func() amboy.Job {
		return makeHostStatsCollector()
	})
}

type hostStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewHostStatsCollector logs statistics about host utilization per
// distro to the default grip logger.
func NewHostStatsCollector(id string) amboy.Job {
	j := makeHostStatsCollector()
	j.SetID(fmt.Sprintf("%s-%s", hostStatsCollectorJobName, id))

	return j
}

func makeHostStatsCollector() *hostStatsCollector {
	j := &hostStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostStatsCollectorJobName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *hostStatsCollector) Run(ctx context.Context) {
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

	j.AddError(j.statsByDistro())
	j.AddError(j.statsByProvider())
}

type hostCountStats struct {
	hosts  *host.DistroStats
	tasks  int
	count  int
	excess int
}

func collectHostCountStats() (*hostCountStats, error) {
	hosts, err := host.GetStatsByDistro()
	if err != nil {
		return nil, errors.Wrap(err, "getting host stats by distro")
	}

	tasks := 0
	count := 0
	excess := 0
	for _, h := range hosts {
		count += h.Count
		tasks += h.NumTasks

		overage := -1 * (h.MaxHosts - h.Count)
		if overage > 0 {
			excess += overage
		}
	}

	stats := &hostCountStats{
		hosts:  &hosts,
		tasks:  tasks,
		count:  count,
		excess: excess,
	}

	return stats, nil
}

func (j *hostStatsCollector) statsByDistro() error {
	stats, err := collectHostCountStats()

	if err != nil {
		return err
	}

	j.logger.Info(message.Fields{
		"report":        "host stats by distro",
		"hosts_total":   stats.count,
		"running_tasks": stats.tasks,
		"data":          stats.hosts,
	})

	j.logger.WarningWhen(stats.excess > 0, message.Fields{
		"report": "maxHosts exceeded",
		"data":   stats.hosts.MaxHostsExceeded(),
		"total":  stats.excess,
	})

	return nil
}

func (j *hostStatsCollector) statsByProvider() error {
	providers, err := host.GetProviderCounts()
	if err != nil {
		return errors.Wrap(err, "getting host stats by provider")
	}

	j.logger.Info(message.Fields{
		"report":    "host stats by provider",
		"providers": providers,
	})

	return nil
}
