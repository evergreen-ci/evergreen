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
	volumeMigrationJobName = "volume-migration"
)

func init() {
	registry.AddJobType(volumeMigrationJobName,
		func() amboy.Job { return makeVolumeMigrationJob() })
}

type volumeMigrationJob struct {
	job.Base      `bson:"job_base" json:"job_base" yaml:"job_base"`
	VolumeID      string             `bson:"volume_id" yaml:"volume_id"`
	ModifyOptions cloud.SpawnOptions `bson:"modify_options" json:"modify_options" yaml:"modify_options"`

	VolumeDetached        bool   `bson:"volume_detached"`
	InitialHostID         string `bson:"initial_host_id"`
	SpawnhostStopJobID    string `bson:"spawnhost_stop_job_id"`
	InitialHostStopped    bool   `bson:"initial_host_stopped"`
	InitialHostTerminated bool   `bson:"initial_host_terminated"`
	NewHostID             string `bson:"new_host_id"`

	env         evergreen.Environment
	volume      *host.Volume
	initialHost *host.Host
}

func makeVolumeMigrationJob() *volumeMigrationJob {
	j := &volumeMigrationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeMigrationJobName,
				Version: 0,
			},
		},
	}
	return j
}

func NewVolumeMigrationJob(env evergreen.Environment, volumeID string, modifyOptions cloud.SpawnOptions, ts string) amboy.Job {
	j := makeVolumeMigrationJob()
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", volumeMigrationJobName, volumeID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", volumeMigrationJobName, volumeID)})
	j.SetEnqueueAllScopes(true)
	j.VolumeID = volumeID
	j.ModifyOptions = modifyOptions
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(50),
		// WaitUntil:   utility.ToTimeDurationPtr(20 * time.Second),
	})

	return j
}

// stopInitialHost inspects the initial host's status
// It returns a boolean indicating whether the host has stopped and a boolean indicating whether an error was added to the job.
func (j *volumeMigrationJob) stopInitialHost(ctx context.Context) (bool, bool) {
	if j.SpawnhostStopJobID == "" {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		stopJob := NewSpawnhostStopJob(j.initialHost, evergreen.User, ts)
		err := j.env.RemoteQueue().Put(ctx, stopJob)
		if err != nil {
			j.AddRetryableError(err)
			return false, true
		}
		grip.Info(message.Fields{
			"message":  "stopping initial host",
			"attempts": j.RetryInfo().CurrentAttempt,
			"host_id":  j.InitialHostID,
		})
		j.SpawnhostStopJobID = stopJob.ID()
		return false, false
	}

	if j.initialHost.Status == evergreen.HostStopped {
		grip.Info(message.Fields{
			"message":  "initial host stopped",
			"attempts": j.RetryInfo().CurrentAttempt,
			"host_id":  j.InitialHostID,
		})
		j.InitialHostStopped = true
		return true, false
	}

	stopJob, foundJob := j.env.RemoteQueue().Get(ctx, j.SpawnhostStopJobID)
	if !foundJob {
		j.AddRetryableError(errors.Errorf("spawnhost-stop job '%s' could not be found", j.SpawnhostStopJobID))
		return false, true
	}

	if stopJob.Status().Completed {
		j.AddError(errors.Errorf("host '%s' failed to stop", j.SpawnhostStopJobID))
		return false, true
	}

	return false, false
}

// startNewHost attempts to start a new host with the volume attached
// It returns a boolean indicating if an error was encountered
func (j *volumeMigrationJob) startNewHost(ctx context.Context) bool {
	// Ensure volume has been detached
	if j.volume.Host == j.InitialHostID {
		grip.Info(message.Fields{
			"message":     "volume still attached to initial host",
			"volume_host": j.volume.Host,
		})
		j.AddRetryableError(errors.New("volume still attached to host"))
		return true
	}

	j.ModifyOptions.HomeVolumeID = j.VolumeID
	intentHost, err := cloud.CreateSpawnHost(ctx, j.ModifyOptions, j.env.Settings())
	if err != nil {
		j.AddError(errors.Wrapf(err, "creating new intent host"))
		return true
	}

	if err := intentHost.Insert(); err != nil {
		j.AddError(errors.Wrap(err, "inserting intent host"))
		return true
	}
	grip.Info(message.Fields{
		"message":  "new intent host created",
		"attempts": j.RetryInfo().CurrentAttempt,
	})
	j.NewHostID = intentHost.Id
	return false
}

func (j *volumeMigrationJob) restartInitialHost(ctx context.Context) bool {
	if j.volume.Host == "" {
		_, err := cloud.AttachVolume(ctx, j.VolumeID, j.InitialHostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "reattaching volume to initial host"))
			return true

		}
	}

	ts := utility.RoundPartOfMinute(1).Format(TSFormat)
	if err := j.env.RemoteQueue().Put(ctx, NewSpawnhostStartJob(j.initialHost, evergreen.User, ts)); err != nil {
		j.AddError(errors.Wrap(err, "enqueuing job to restart initial host"))
		return true
	}
	return false

}

func (j *volumeMigrationJob) finishJob(ctx context.Context) {
	// If this is the job's final attempt or a non-retryable error has been encountered, do some cleanup
	// Amboy's ShouldRetry() method does not account for reaching MaxAttempts, so manually determine if the current attempt is the last
	retryInfo := j.RetryInfo()
	isFinalAttempt := retryInfo.CurrentAttempt == (retryInfo.MaxAttempts - 1)
	if !retryInfo.ShouldRetry() || isFinalAttempt {
		// If new host was never created, restart initial host with volume attached
		if j.NewHostID == "" {
			if hasErr := j.restartInitialHost(ctx); hasErr {
				return
			}
		}

		if err := j.volume.SetMigrating(false); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "marking volume as done migrating",
				"volume_id": j.volume.ID,
			}))
			j.AddRetryableError(err)
			return
		}

	}

	j.MarkComplete()
}

func (j *volumeMigrationJob) Run(ctx context.Context) {
	defer j.finishJob(ctx)

	if err := j.populateIfUnset(); err != nil {
		j.AddRetryableError(err)
		return
	}

	if err := j.volume.SetMigrating(true); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "marking volume as migrating",
			"host_id": j.volume.ID,
			"job":     j.ID(),
		}))
		j.AddRetryableError(err)
		return
	}

	if !j.InitialHostStopped {
		initialHostStopped, hasErr := j.stopInitialHost(ctx)
		if hasErr {
			return
		}
		if !initialHostStopped {
			j.AddRetryableError(errors.Errorf("initial host '%s' not yet stopped", j.InitialHostID))
			return
		}
	}

	if !j.VolumeDetached {
		// Unmount volume from initial host
		if err := j.initialHost.UnsetHomeVolume(); err != nil {
			j.AddError(errors.Wrapf(err, "unsetting home volume '%s' from host '%s'", j.VolumeID, j.InitialHostID))
			return
		}

		if _, err := cloud.DetachVolume(ctx, j.VolumeID); err != nil {
			j.AddError(errors.Wrapf(err, "detaching volume '%s'", j.VolumeID))
			return
		}
		j.VolumeDetached = true

		// Update in-memory volume
		volume, err := j.getVolume()
		if err != nil {
			j.AddError(err)
			return
		}
		j.volume = volume
	}

	// Avoid recreating new host on retry
	if j.NewHostID == "" {
		if hasErr := j.startNewHost(ctx); hasErr {
			return
		}
	}

	// Terminate initial host
	if !j.InitialHostTerminated {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		terminateJob := NewSpawnHostTerminationJob(j.initialHost, evergreen.User, ts)
		if err := j.env.RemoteQueue().Put(ctx, terminateJob); err != nil {
			j.AddRetryableError(errors.Wrapf(err, "enqueueing initial host termination job '%s'", terminateJob.ID()))
			return
		}
		j.InitialHostTerminated = true
	}
}

func (j *volumeMigrationJob) getVolume() (*host.Volume, error) {
	volume, err := host.FindVolumeByID(j.VolumeID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding volume '%s'", j.VolumeID)
	}
	if volume == nil {
		return nil, errors.Errorf("volume '%s' not found", j.VolumeID)
	}
	return volume, nil
}

func (j *volumeMigrationJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.volume == nil {
		volume, err := j.getVolume()
		if err != nil {
			return err
		}
		j.volume = volume
	}

	if j.InitialHostID == "" {
		j.InitialHostID = j.volume.Host
	}

	if j.initialHost == nil {
		initialHost, err := host.FindOneId(j.InitialHostID)
		if err != nil {
			return errors.Wrapf(err, "getting host with ID '%s'", j.InitialHostID)
		}
		if initialHost == nil {
			return errors.Errorf("host '%s' not found", j.InitialHostID)
		}
		j.initialHost = initialHost
	}

	return nil
}
