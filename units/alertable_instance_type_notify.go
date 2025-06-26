package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const alertableInstanceTypeNotifyJobName = "alertable-instance-type-notify"

func init() {
	registry.AddJobType(alertableInstanceTypeNotifyJobName, func() amboy.Job {
		return makeAlertableInstanceTypeNotifyJob()
	})
}

type alertableInstanceTypeNotifyJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeAlertableInstanceTypeNotifyJob() *alertableInstanceTypeNotifyJob {
	j := &alertableInstanceTypeNotifyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    alertableInstanceTypeNotifyJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewAlertableInstanceTypeNotifyJob creates a job to check for spawn hosts using alertable instance types
func NewAlertableInstanceTypeNotifyJob(id string) amboy.Job {
	j := makeAlertableInstanceTypeNotifyJob()
	j.SetID(fmt.Sprintf("%s.%s", alertableInstanceTypeNotifyJobName, id))
	return j
}

func (j *alertableInstanceTypeNotifyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting evergreen settings"))
		return
	}

	alertableTypes := settings.Providers.AWS.AlertableInstanceTypes
	if len(alertableTypes) == 0 {
		return
	}

	// Find all active spawn hosts
	hosts, err := host.Find(ctx, host.ByUnterminatedSpawnHostsWithInstanceTypes(alertableTypes))
	if err != nil {
		j.AddError(errors.Wrap(err, "finding spawn hosts"))
		return
	}

	hostsAlerted := []string{}
	hostsFound := []string{}

	// Check each host for alertable instance types and trigger notifications
	const alertThreshold = 72 * time.Hour
	for _, h := range hosts {
		// Check if this host has been using an alertable instance type for longer than 3 days
		if utility.IsZeroTime(h.LastInstanceEditTime) || (time.Since(h.LastInstanceEditTime) >= alertThreshold) {
			alerted, err := tryAlertableInstanceTypeNotification(ctx, &h)
			if err != nil {
				j.AddError(errors.Wrap(err, "logging events for alertable instance type"))
				grip.Error(message.WrapError(err, message.Fields{
					"runner":  "monitor",
					"id":      j.ID(),
					"message": "error queuing alert",
					"host_id": h.Id,
				}))
			}
			hostsFound = append(hostsFound, h.Id)
			if alerted {
				hostsAlerted = append(hostsAlerted, h.Id)
			}
		}
	}

	grip.Info(message.Fields{
		"job":             alertableInstanceTypeNotifyJobName,
		"message":         "finished running alertable instance type notify job",
		"num_hosts_found": len(hosts),
		"hosts_found":     hostsFound,
		"hosts_alerted":   hostsAlerted,
		"alertable_types": alertableTypes,
	})
}

func shouldNotifyForAlertableInstanceType(ctx context.Context, h *host.Host) (bool, error) {
	rec, err := alertrecord.FindByMostRecentAlertableInstanceType(ctx, h.Id)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return true, nil
	}

	return time.Since(rec.AlertTime) > hostRenotificationInterval, nil
}

// tryAlertableInstanceTypeNotification returns true if a notification was attempted.
func tryAlertableInstanceTypeNotification(ctx context.Context, h *host.Host) (bool, error) {
	shouldExec, err := shouldNotifyForAlertableInstanceType(ctx, h)
	if err != nil {
		return false, err
	}
	if !shouldExec {
		return false, nil
	}

	// Insert subscriptions for the user, if needed.
	usr, err := user.FindOneByIdContext(ctx, h.StartedBy)
	if err != nil {
		return false, errors.Wrapf(err, "finding user '%s'", h.StartedBy)
	}
	if usr == nil {
		return false, errors.Errorf("user '%s' not found", h.StartedBy)
	}

	emailSubscriber := event.NewEmailSubscriber(usr.Email())
	emailSubscription := event.NewAlertableInstanceTypeWarningSubscription(h.Id, emailSubscriber)
	if err = emailSubscription.Upsert(ctx); err != nil {
		return false, errors.Wrap(err, "upserting alertable instance type email subscription")
	}

	slackTarget := fmt.Sprintf("@%s", usr.Settings.SlackUsername)
	if usr.Settings.SlackMemberId != "" {
		slackTarget = usr.Settings.SlackMemberId
	}
	slackSubscriber := event.NewSlackSubscriber(slackTarget)
	slackSubscription := event.NewAlertableInstanceTypeWarningSubscription(h.Id, slackSubscriber)
	if err = slackSubscription.Upsert(ctx); err != nil {
		return false, errors.Wrap(err, "upserting alertable instance type slack subscription")
	}

	event.LogAlertableInstanceTypeWarningSent(ctx, h.Id)
	return true, alertrecord.InsertNewAlertableInstanceTypeRecord(ctx, h.Id)
}
