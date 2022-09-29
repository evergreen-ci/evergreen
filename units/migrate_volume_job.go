package units

import (
	"context"
	"fmt"
	"time"

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

	VolumeDetached     bool   `bson:"volume_detached"`
	InitialHostID      string `bson:"initial_host_id"`
	SpawnhostStopJobID string `bson:"spawnhost_stop_job_id"`
	NewHostID          string `bson:"new_host_id"`

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
		MaxAttempts: utility.ToIntPtr(30),
		WaitUntil:   utility.ToTimeDurationPtr(20 * time.Second),
	})

	return j
}

func (j *volumeMigrationJob) Run(ctx context.Context) {
	defer j.finishJob(ctx)

	if err := j.populateIfUnset(); err != nil {
		j.AddRetryableError(err)
		return
	}

	if err := j.volume.SetMigrating(true); err != nil {
		j.AddRetryableError(err)
		return
	}

	if j.initialHost.Status != evergreen.HostStopped || j.initialHost.Status != evergreen.HostTerminated {
		j.stopInitialHost(ctx)
		if j.HasErrors() {
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
		volume, err := host.FindVolumeByID(j.VolumeID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding volume '%s'", j.VolumeID))
			return
		}
		if volume == nil {
			j.AddError(errors.Errorf("volume '%s' not found", j.VolumeID))
			return
		}
		j.volume = volume
	}

	// Avoid recreating new host on retry
	if j.NewHostID == "" {
		j.startNewHost(ctx)
		if j.HasErrors() {
			return
		}
	}

	// Terminate initial host
	ts := utility.RoundPartOfMinute(1).Format(TSFormat)
	terminateJob := NewSpawnHostTerminationJob(j.initialHost, evergreen.User, ts)
	if err := j.env.RemoteQueue().Put(ctx, terminateJob); err != nil {
		j.AddRetryableError(errors.Wrapf(err, "enqueueing initial host termination job '%s'", terminateJob.ID()))
		return
	}
}

// stopInitialHost inspects the initial host's status and stops it if the host is still running.
func (j *volumeMigrationJob) stopInitialHost(ctx context.Context) {
	// Stop host if this operation has not been attempted yet
	if j.SpawnhostStopJobID == "" {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		stopJob := NewSpawnhostStopJob(j.initialHost, evergreen.User, ts)
		err := j.env.RemoteQueue().Put(ctx, stopJob)
		if err != nil {
			j.AddRetryableError(err)
			return
		}
		grip.Info(message.Fields{
			"message":         "stopping initial host",
			"job_id":          j.ID(),
			"initial_host_id": j.InitialHostID,
		})
		j.SpawnhostStopJobID = stopJob.ID()
		j.AddRetryableError(errors.Errorf("initial host '%s' not yet stopped", j.InitialHostID))
		return
	}

	// Mark host as stopped
	if j.initialHost.Status == evergreen.HostStopped {
		grip.Info(message.Fields{
			"message":         "initial host stopped",
			"job_id":          j.ID(),
			"initial_host_id": j.InitialHostID,
		})
		return
	}

	// If host is not stopped, verify whether the job is still in progress and error if it completed without stopping.
	stopJob, foundJob := j.env.RemoteQueue().Get(ctx, j.SpawnhostStopJobID)
	if !foundJob {
		j.AddError(errors.Errorf("spawnhost-stop job '%s' could not be found", j.SpawnhostStopJobID))
		return
	}

	if stopJob.Status().Completed {
		j.AddError(errors.Errorf("host '%s' failed to stop", j.SpawnhostStopJobID))
		return
	}

	j.AddRetryableError(errors.Errorf("initial host '%s' not yet stopped", j.InitialHostID))
}

// startNewHost attempts to start a new host with the volume attached.
func (j *volumeMigrationJob) startNewHost(ctx context.Context) {
	// Ensure volume has been detached
	if j.volume.Host == j.InitialHostID {
		j.AddRetryableError(errors.New("volume still attached to host"))
		return
	}

	j.ModifyOptions.HomeVolumeID = j.VolumeID
	intentHost, err := cloud.CreateSpawnHost(ctx, j.ModifyOptions, j.env.Settings())
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "creating new intent host"))
		return
	}

	if err := intentHost.Insert(); err != nil {
		j.AddError(errors.Wrap(err, "inserting new intent host"))
		return
	}
	grip.Info(message.Fields{
		"message":        "new intent host created",
		"job_id":         j.ID(),
		"intent_host_id": intentHost.Id,
	})
	j.NewHostID = intentHost.Id
	return
}

// finishJob marks the job as completed and attempts some additional cleanup if this is the job's final attempt.
func (j *volumeMigrationJob) finishJob(ctx context.Context) {
	// Amboy's ShouldRetry() method does not account for reaching MaxAttempts, so manually determine if the current attempt is the last.
	retryInfo := j.RetryInfo()
	isFinalAttempt := retryInfo.CurrentAttempt >= (retryInfo.MaxAttempts - 1)
	if !retryInfo.ShouldRetry() || isFinalAttempt {
		// If a new host was never created, restart initial host with volume attached.
		if j.NewHostID == "" {
			if hasErr := j.restartInitialHost(ctx); hasErr {
				return
			}
		}

		if err := j.volume.SetMigrating(false); err != nil {
			j.AddRetryableError(err)
			return
		}

	}

	j.MarkComplete()
}

// restartInitialHost restarts the stopped host with the volume attached.
// This is a fallback so that we can attempt to leave users with a usable workstation in case other operations fail.
// It returns a boolean indicating whether an error was added to the job.
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
		volume, err := host.FindVolumeByID(j.VolumeID)
		if err != nil {
			return errors.Wrapf(err, "finding volume '%s'", j.VolumeID)
		}
		if volume == nil {
			return errors.Errorf("volume '%s' not found", j.VolumeID)
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
