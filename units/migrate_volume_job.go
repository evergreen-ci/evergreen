package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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
	volumeMigrationName = "volume-migration"
)

func init() {
	registry.AddJobType(volumeMigrationName,
		func() amboy.Job { return makeVolumeMigrationJob() })
}

type volumeMigrationJob struct {
	job.Base      `bson:"job_base" json:"job_base" yaml:"job_base"`
	VolumeID      string                 `bson:"volume_id" yaml:"volume_id"`
	ModifyOptions host.HostModifyOptions `bson:"modify_options" json:"modify_options" yaml:"modify_options"`

	env         evergreen.Environment
	volume      *host.Volume
	initialHost *host.Host
	newHost     *host.Host
}

func makeVolumeMigrationJob() *volumeMigrationJob {
	j := &volumeMigrationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeMigrationName,
				Version: 0,
			},
		},
	}
	return j
}

func NewVolumeMigrationJob(env evergreen.Environment, volumeID string, ts string) amboy.Job {
	j := makeVolumeMigrationJob()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", volumeMigrationName, volumeID, ts))
	j.VolumeID = volumeID
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(5),
		// WaitUntil:   utility.ToTimeDurationPtr(5 * time.Second),
	})

	return j
}

func (j *volumeMigrationJob) initialHostStopped(ctx context.Context) (bool, error) {
	switch j.initialHost.Status {
	case evergreen.HostRunning:
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		j.env.RemoteQueue().Put(ctx, NewSpawnhostStopJob(j.initialHost, evergreen.User, ts))
		grip.Info(message.Fields{
			"message":  "stopping initial host",
			"host_id":  j.initialHost.Id,
			"attempts": j.RetryInfo().CurrentAttempt,
			"job":      j.ID(),
		})
		return false, nil
	case evergreen.HostStopped:
		grip.Info(message.Fields{
			"message":  "initial host stopped",
			"host_id":  j.initialHost.Id,
			"attempts": j.RetryInfo().CurrentAttempt,
			"job":      j.ID(),
		})
		return true, nil
	default:
		return false, nil
	}
	return false, nil
}

func (j *volumeMigrationJob) startNewHost(ctx context.Context) {

}

func (j *volumeMigrationJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	grip.Info(message.Fields{
		"message":  "Job running",
		"attempts": j.RetryInfo().CurrentAttempt,
		"job":      j.ID(),
	})

	if err := j.populateIfUnset(); err != nil {
		j.AddRetryableError(err)
		return
	}

	initialHostStopped, err := j.initialHostStopped(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "stopping initial host"))
		return
	}
	if !initialHostStopped {
		j.UpdateRetryInfo(amboy.JobRetryOptions{
			NeedsRetry: utility.TruePtr(),
		})
		return
	}

	// Unmount volume from initial host
	_, err = cloud.DetachVolume(ctx, j.VolumeID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "detaching volume from initial host"))
		return
	}

	/*
		// Spawn new host
		// Add check to ensure that host document (?) doesn't exist already
		newHost, err := cloud.CreateSpawnHost(ctx, cloud.SpawnOptions{}, j.env.Settings())
		if err != nil {
			j.AddError(errors.Wrapf(err, "creating new intent host"))
		}
		j.newHost = newHost

		// Attach volume to host
		if err = j.newHost.SetHomeVolumeID(j.VolumeID); err != nil {
			j.AddError(errors.Wrapf(err, "setting home volume ID for host"))
			return
		}

		// Terminate initial host
		terminateJob := NewHostTerminationJob(j.env, j.initialHost, HostTerminationOptions{
			TerminateIfBusy: true,
		})
		j.AddError(amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), terminateJob)) */

}

func (j *volumeMigrationJob) populateIfUnset() error {
	if j.volume == nil {
		volume, err := host.FindVolumeByID(j.VolumeID)
		if err != nil {
			return errors.Wrapf(err, "finding volume '%s'", j.VolumeID)
		}
		if volume == nil {
			return errors.Errorf("volume '%s' not found", j.VolumeID)
		}
		j.volume = volume
	}

	if j.initialHost == nil {
		initialHost, err := host.FindHostWithVolume(j.VolumeID)
		if err != nil {
			return errors.Wrapf(err, "getting host with volume '%s' attached", j.VolumeID)
		}
		if initialHost == nil {
			return errors.Errorf("volume '%s' is not attached to host", j.VolumeID)
		}
		j.initialHost = initialHost
	}

	return nil
}
