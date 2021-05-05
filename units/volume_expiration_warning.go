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
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	volumeExpirationWarningsName = "volume-expiration-warnings"
)

func init() {
	registry.AddJobType(volumeExpirationWarningsName,
		func() amboy.Job { return makeVolumeExpirationWarningsJob() })
}

type volumeExpirationWarningsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeVolumeExpirationWarningsJob() *volumeExpirationWarningsJob {
	j := &volumeExpirationWarningsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeExpirationWarningsName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewVolumeExpirationWarningsJob(id string) amboy.Job {
	j := makeVolumeExpirationWarningsJob()
	j.SetID(fmt.Sprintf("%s.%s", volumeExpirationWarningsName, id))
	j.SetScopes([]string{volumeExpirationWarningsName})
	j.SetShouldApplyScopesOnEnqueue(true)
	return j
}

func (j *volumeExpirationWarningsJob) Run(ctx context.Context) {
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

	// Do alerts for volumes - collect all volumes that are unattached.
	// The trigger logic will filter out any volumes that aren't in a notification window, or have
	// already have alerts sent.
	unattachedVolumes, err := host.FindUnattachedExpirableVolumes()
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	for _, v := range unattachedVolumes {
		if ctx.Err() != nil {
			j.AddError(errors.New("volume expiration warning run canceled"))
			return
		}
		if err = runVolumeWarningTriggers(v); err != nil {
			j.AddError(err)
			grip.Error(message.WrapError(err, message.Fields{
				"runner":    "monitor",
				"id":        j.ID(),
				"message":   "Error queuing alert",
				"volume_id": v.ID,
			}))
		}
	}
}

func runVolumeWarningTriggers(v host.Volume) error {
	catcher := grip.NewSimpleCatcher()
	// try notifying with the largest notification types first.
	// If an alert has been sent, don't continue to the smaller types.
	triggerHours := []int{24 * 21, 24 * 14, 24 * 7, 12, 2}
	for _, numHours := range triggerHours {
		ok, err := tryVolumeNotification(v, numHours)
		catcher.Add(err)
		if ok {
			return catcher.Resolve()
		}
	}
	return catcher.Resolve()
}

// return true if a notification was added
func tryVolumeNotification(v host.Volume, numHours int) (bool, error) {
	shouldExec, err := shouldNotifyForVolumeExpiration(v, numHours)
	if err != nil {
		return false, err
	}
	if !shouldExec {
		return false, nil
	}
	event.LogVolumeExpirationWarningSent(v.ID)
	if err = alertrecord.InsertNewVolumeExpirationRecord(v.ID, numHours); err != nil {
		return false, err
	}
	return true, nil
}

func shouldNotifyForVolumeExpiration(v host.Volume, numHours int) (bool, error) {
	numHoursLeft := v.Expiration.Sub(time.Now())
	// say we have 15 hours left. if 15 > 12, quit. if 15 > 2,  quit.
	// say we have 10 hours left. 10 > 12 nope, so send. 12 > 2 so quit.
	// say we have 30 days left. greater than everything, send no notices.
	// say we have 5 days left. greater than 14 days and 21 days.
	if numHoursLeft > (time.Duration(numHours) * time.Hour) {
		return false, nil
	}
	rec, err := alertrecord.FindByVolumeExpirationWithHours(v.ID, numHours)
	if err != nil {
		return false, err
	}

	return rec == nil, nil
}
