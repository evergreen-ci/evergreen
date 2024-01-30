package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	unexpirableSpawnHostStatsJobName        = "unexpirable-spawnhost-stats"
	unexpirableSpawnHostStatsJobMaxAttempts = 10
)

func init() {
	registry.AddJobType(unexpirableSpawnHostStatsJobName, func() amboy.Job {
		return makeUnexpirableSpawnHostStatsJob()
	})
}

type unexpirableSpawnHostStatsJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeUnexpirableSpawnHostStatsJob() *unexpirableSpawnHostStatsJob {
	j := &unexpirableSpawnHostStatsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    unexpirableSpawnHostStatsJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewUnexpirableSpawnHostStatsJob returns a job to collect estimated statistics
// on unexpirable spawn host usage.
func NewUnexpirableSpawnHostStatsJob(ts string) amboy.Job {
	j := makeUnexpirableSpawnHostStatsJob()
	j.SetID(fmt.Sprintf("%s.%s", unexpirableSpawnHostStatsJobName, ts))
	j.SetScopes([]string{unexpirableSpawnHostStatsJobName})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(unexpirableSpawnHostStatsJobMaxAttempts),
	})
	return j
}

func (j *unexpirableSpawnHostStatsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	hosts, err := host.FindUnexpirableRunning()
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "finding unexpirable running hosts"))
		return
	}

	stats := j.getStats(hosts)

	grip.Info(message.Fields{
		"message":                      "unexpirable spawn host stats",
		"job_id":                       j.ID(),
		"total_uptime_secs":            stats.totalUptime.Seconds(),
		"uptime_secs_by_distro":        stats.uptimeSecsByDistro,
		"uptime_secs_by_instance_type": stats.uptimeSecsByInstanceType,
	})
}

type unexpirableSpawnHostStats struct {
	totalUptime              time.Duration
	uptimeSecsByDistro       map[string]int
	uptimeSecsByInstanceType map[string]int
}

// getStats returns the estimated host uptime stats for the entire day. These
// are not perfectly accurate, but give a sufficient ballpark estimate of spawn
// host uptime. For example, it's assumed for simplicity that users don't stop
// their hosts throughout the day, so if the host is on, it's likely been on the
// entire day.
func (j *unexpirableSpawnHostStatsJob) getStats(hosts []host.Host) unexpirableSpawnHostStats {
	var totalUptime time.Duration
	uptimeByDistro := map[string]int{}
	uptimeByInstanceType := map[string]int{}

	for _, h := range hosts {
		// Estimate that if the host is up now, it's been up all day.
		const dailyUptimePerHost = 24 * time.Hour
		totalUptime += dailyUptimePerHost
		uptimeByDistro[h.Distro.Id] += int(dailyUptimePerHost.Seconds())
		if evergreen.IsEc2Provider(h.Distro.Provider) {
			instanceType := h.InstanceType
			if instanceType == "" {
				continue
			}
			uptimeByInstanceType[instanceType] += int(dailyUptimePerHost.Seconds())
		}
	}

	return unexpirableSpawnHostStats{
		totalUptime:              totalUptime,
		uptimeSecsByDistro:       uptimeByDistro,
		uptimeSecsByInstanceType: uptimeByInstanceType,
	}
}
