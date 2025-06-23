package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
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
	env      evergreen.Environment
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

	// Check each host for alertable instance types and trigger notifications
	const alertThreshold = 72 * time.Hour
	for _, h := range hosts {
		// Check if this host has been using an alertable instance type for longer than 3 days
		if h.LastInstanceEditTime.IsZero() || (time.Since(h.LastInstanceEditTime) >= alertThreshold) {
			if err = tryAlertableInstanceTypeNotification(ctx, &h); err != nil {
				j.AddError(errors.Wrap(err, "logging events for alertable instance type"))
				grip.Error(message.WrapError(err, message.Fields{
					"runner":  "monitor",
					"id":      j.ID(),
					"message": "Error queuing alert",
					"host_id": h.Id,
				}))
			}
		}
	}
}

func shouldNotifyForAlertableInstanceType(ctx context.Context, h *host.Host) (bool, error) {
	// Use a fixed "daily" alert type since we want daily reminders, not hour-based thresholds
	rec, err := alertrecord.FindByMostRecentAlertableInstanceTypeWithHours(ctx, h.Id, 0)
	if err != nil {
		return false, err
	}
	if rec == nil {
		return true, nil
	}

	return time.Since(rec.AlertTime) > hostRenotificationInterval, nil
}

func tryAlertableInstanceTypeNotification(ctx context.Context, h *host.Host) error {
	shouldExec, err := shouldNotifyForAlertableInstanceType(ctx, h)
	if err != nil {
		return err
	}
	if !shouldExec {
		return nil
	}

	event.LogAlertableInstanceTypeWarningSent(ctx, h.Id)
	grip.Info(message.Fields{
		"message":       "sent alertable instance type warning",
		"host_id":       h.Id,
		"owner":         h.StartedBy,
		"instance_type": h.InstanceType,
	})
	// Use 0 as a fixed identifier for daily alertable instance type notifications
	return alertrecord.InsertNewAlertableInstanceTypeRecord(ctx, h.Id, 0)
}
