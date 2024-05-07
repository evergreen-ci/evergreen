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
		"message":           "unexpirable spawn host stats",
		"job_id":            j.ID(),
		"total_uptime_secs": stats.totalUptime.Seconds(),
	})

	for distroID, uptimeSecs := range stats.uptimeSecsByDistro {
		grip.Info(message.Fields{
			"message":     "unexpirable spawn host stats by distro",
			"job_id":      j.ID(),
			"distro":      distroID,
			"uptime_secs": uptimeSecs,
		})
	}
	for instanceType, uptimeSecs := range stats.uptimeSecsByInstanceType {
		grip.Info(message.Fields{
			"message":       "unexpirable spawn host stats by instance type",
			"job_id":        j.ID(),
			"instance_type": instanceType,
			"uptime_secs":   uptimeSecs,
		})
	}
}

type unexpirableSpawnHostStats struct {
	totalUptime              time.Duration
	uptimeSecsByDistro       map[string]int
	uptimeSecsByInstanceType map[string]int
}

// getStats returns the estimated host uptime stats for the hour. These are not
// perfectly accurate, but give a sufficient ballpark estimate of spawn host
// uptime. It's assumed for simplicity that users don't stop/start their hosts
// many times per day, which would alter the accuracy of the stats.
func (j *unexpirableSpawnHostStatsJob) getStats(hosts []host.Host) unexpirableSpawnHostStats {
	var totalUptime time.Duration
	uptimeByDistro := map[string]int{}
	uptimeByInstanceType := map[string]int{}

	for _, h := range hosts {
		// This job runs once per hour. If the host is up now, estimate that
		// it's been up for the past hour.
		const hourlyUptimePerHost = time.Hour
		totalUptime += hourlyUptimePerHost
		uptimeByDistro[h.Distro.Id] += int(hourlyUptimePerHost.Seconds())
		if evergreen.IsEc2Provider(h.Distro.Provider) {
			instanceType := h.InstanceType
			if instanceType == "" {
				continue
			}
			uptimeByInstanceType[instanceType] += int(hourlyUptimePerHost.Seconds())
		}
	}

	return unexpirableSpawnHostStats{
		totalUptime:              totalUptime,
		uptimeSecsByDistro:       uptimeByDistro,
		uptimeSecsByInstanceType: uptimeByInstanceType,
	}
}
