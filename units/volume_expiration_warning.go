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
	j := makeSpawnhostExpirationWarningsJob()
	j.SetID(fmt.Sprintf("%s.%s", volumeExpirationWarningsName, id))
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

	// Do alerts for volumes - collect all volumes expiring in the next 12 hours.
	// The trigger logic will filter out any volumes that aren't in a notification window, or have
	// already have alerts sent.
	thresholdTime := time.Now().Add(12 * time.Hour)
	expiringSoonVolumes, err := host.FindVolumesToDelete(thresholdTime)
	if err != nil {
		j.AddError(errors.WithStack(err))
		return
	}

	for _, v := range expiringSoonVolumes {
		if ctx.Err() != nil {
			j.AddError(errors.New("spawnhost expiration warning run canceled"))
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
	catcher.Add(tryVolumeNotifcation(v, 2))
	catcher.Add(tryVolumeNotifcation(v, 12))
	return catcher.Resolve()
}

func tryVolumeNotifcation(v host.Volume, numHours int) error {
	shouldExec, err := shouldNotifyForVolumeExpiration(v, numHours)
	if err != nil {
		return err
	}
	if shouldExec {
		event.LogVolumeExpirationWarningSent(v.ID)
		if err = alertrecord.InsertNewVolumeExpirationRecord(v.ID, numHours); err != nil {
			return err
		}
	}
	return nil
}

func shouldNotifyForVolumeExpiration(v host.Volume, numHours int) (bool, error) {
	if v.Expiration.Sub(time.Now()) > (time.Duration(numHours) * time.Hour) {
		return false, nil
	}
	rec, err := alertrecord.FindByVolumeExpirationWithHours(v.ID, numHours)
	if err != nil {
		return false, err
	}

	return rec == nil, nil
}
