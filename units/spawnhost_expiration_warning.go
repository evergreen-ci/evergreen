package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const spawnhostExpirationWarningsName = "spawnhost-expiration-warnings"

func init() {
	registry.AddJobType(spawnhostExpirationWarningsName,
		func() amboy.Job { return makeSpawnhostExpirationWarningsJob() })
}

type spawnhostExpirationWarningsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnhostExpirationWarningsJob() *spawnhostExpirationWarningsJob {
	j := &spawnhostExpirationWarningsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostExpirationWarningsName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostExpirationWarningsJob(id string) amboy.Job {
	j := makeSpawnhostExpirationWarningsJob()
	j.SetID(fmt.Sprintf("%s.%s", spawnhostExpirationWarningsName, id))
	return j
}

func (j *spawnhostExpirationWarningsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.AlertsDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  "alerter",
			"id":      j.ID(),
			"message": "alerts are disabled, exiting",
		})
		return
	}

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
		if ctx.Err() != nil {
			j.AddError(errors.New("spawnhost expiration warning run canceled"))
			return
		}
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
