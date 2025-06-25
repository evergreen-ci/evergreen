package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	spawnhostExpirationWarningsName = "spawnhost-expiration-warnings"
)

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
	return j
}

func NewSpawnhostExpirationWarningsJob(id string) amboy.Job {
	j := makeSpawnhostExpirationWarningsJob()
	j.SetID(fmt.Sprintf("%s.%s", spawnhostExpirationWarningsName, id))
	j.SetScopes([]string{spawnhostExpirationWarningsName})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *spawnhostExpirationWarningsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
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
	expiringSoonHosts, err := host.Find(ctx, host.ByExpiringBetween(now, thresholdTime))
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	for _, h := range expiringSoonHosts {
		if ctx.Err() != nil {
			j.AddError(errors.Wrap(ctx.Err(), "spawnhost expiration warning run canceled"))
			return
		}
		if err = runSpawnHostExpirationWarningTriggers(ctx, &h); err != nil {
			j.AddError(errors.Wrap(err, "logging events for spawn host expiration"))
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  "monitor",
				"id":      j.ID(),
				"message": "error queuing alert",
				"host_id": h.Id,
			}))
		}
	}

	// Notify for spawn host temporary exemptions expiring soon.
	temporaryExemptionExpiringSoonHosts, err := host.FindByTemporaryExemptionsExpiringBetween(ctx, now, thresholdTime)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding hosts with temporary exemptions expiring soon"))
		return
	}
	for _, h := range temporaryExemptionExpiringSoonHosts {
		if ctx.Err() != nil {
			j.AddError(errors.Wrap(ctx.Err(), "temporary exemption expiration warning run canceled"))
			return
		}
		if err = runHostTemporaryExemptionExpirationWarningTriggers(ctx, &h); err != nil {
			j.AddError(errors.Wrap(err, "logging events for temporary exemption expiration"))
		}
	}
}

func shouldNotifyForSpawnhostExpiration(ctx context.Context, h *host.Host, numHours int) (bool, error) {
	if h == nil || h.ExpirationTime.IsZero() || time.Until(h.ExpirationTime) > (time.Duration(numHours)*time.Hour) {
		return false, nil
	}
	rec, err := alertrecord.FindByMostRecentSpawnHostExpirationWithHours(ctx, h.Id, numHours)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return true, nil
	}

	return time.Since(rec.AlertTime) > hostRenotificationInterval, nil
}

// hostRenotificationInterval is how frequently a host-related notification can
// be re-sent after one has already been sent.
const hostRenotificationInterval = utility.Day

func shouldNotifyForHostTemporaryExemptionExpiration(ctx context.Context, h *host.Host, numHours int) (bool, error) {
	if utility.IsZeroTime(h.SleepSchedule.TemporarilyExemptUntil) || time.Until(h.SleepSchedule.TemporarilyExemptUntil) > time.Duration(numHours)*time.Hour {
		return false, nil
	}
	rec, err := alertrecord.FindByMostRecentTemporaryExemptionExpirationWithHours(ctx, h.Id, numHours)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return true, nil
	}

	return time.Since(rec.AlertTime) > hostRenotificationInterval, nil
}

func trySpawnHostExpirationNotification(ctx context.Context, h *host.Host, numHours int) error {
	shouldExec, err := shouldNotifyForSpawnhostExpiration(ctx, h, numHours)
	if err != nil {
		return err
	}
	if shouldExec {
		event.LogSpawnhostExpirationWarningSent(ctx, h.Id)
		grip.Info(message.Fields{
			"message":    "sent host expiration warning",
			"host_id":    h.Id,
			"owner":      h.StartedBy,
			"expiration": h.ExpirationTime,
		})
		if err = alertrecord.InsertNewSpawnHostExpirationRecord(ctx, h.Id, numHours); err != nil {
			return err
		}
	}
	return nil
}

func tryHostTemporaryExemptionExpirationNotification(ctx context.Context, h *host.Host, numHours int) error {
	shouldExec, err := shouldNotifyForHostTemporaryExemptionExpiration(ctx, h, numHours)
	if err != nil {
		return err
	}
	if shouldExec {
		event.LogHostTemporaryExemptionExpirationWarningSent(ctx, h.Id)
		grip.Info(message.Fields{
			"message": "sent temporary exemption expiration warning",
			"host_id": h.Id,
			"owner":   h.StartedBy,
			"until":   h.SleepSchedule.TemporarilyExemptUntil,
		})
		if err = alertrecord.InsertNewHostTemporaryExemptionExpirationRecord(ctx, h.Id, numHours); err != nil {
			return err
		}
	}
	return nil
}

func runSpawnHostExpirationWarningTriggers(ctx context.Context, h *host.Host) error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(trySpawnHostExpirationNotification(ctx, h, 2))
	catcher.Add(trySpawnHostExpirationNotification(ctx, h, 12))
	return catcher.Resolve()
}

func runHostTemporaryExemptionExpirationWarningTriggers(ctx context.Context, h *host.Host) error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(tryHostTemporaryExemptionExpirationNotification(ctx, h, 2))
	catcher.Add(tryHostTemporaryExemptionExpirationNotification(ctx, h, 12))
	return catcher.Resolve()
}
