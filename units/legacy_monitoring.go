package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/monitor"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const legacyMonitorRunnerJobName = "legacy-monitor-runner"

func init() {
	registry.AddJobType(legacyMonitorRunnerJobName,
		func() amboy.Job { return makeLegacyMonitorRunnerJob() })
}

type legacyMonitorJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeLegacyMonitorRunnerJob() *legacyMonitorJob {
	return &legacyMonitorJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    legacyMonitorRunnerJobName,
				Version: 0,
			},
		},
	}
}

func NewLegacyMonitorRunnerJob(env evergreen.Environment, id string) amboy.Job {
	j := makeLegacyMonitorRunnerJob()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", legacyMonitorRunnerJobName, id))
	return j
}

func (j *legacyMonitorJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	config := j.env.Settings()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.MonitorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  "monitor",
			"id":      j.ID(),
			"message": "monitor is disabled, exiting",
		})
		return
	}

	notifier := &monitor.Notifier{
		NotificationBuilders: []monitor.NotificationBuilder{
			monitor.SpawnHostExpirationWarnings,
			monitor.SlowProvisioningWarnings,
		},
	}

	// send notifications
	grip.Error(message.WrapError(notifier.Notify(config), message.Fields{
		"runner":  "monitor",
		"id":      j.ID(),
		"message": "Error sending notifications",
	}))

	// Do alerts for spawnhosts - collect all hosts expiring in the next 12 hours.
	// The trigger logic will filter out any hosts that aren't in a notification window, or have
	// already have alerts sent.
	now := time.Now()
	thresholdTime := now.Add(12 * time.Hour)
	expiringSoonHosts, err := host.Find(host.ByExpiringBetween(now, thresholdTime))
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	for _, h := range expiringSoonHosts {
		if err = alerts.RunSpawnWarningTriggers(&h); err != nil {
			j.AddError(err)
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  "monitor",
				"id":      j.ID(),
				"message": "Error queuing alert",
				"host":    h.Id,
			}))
		}
	}

}
