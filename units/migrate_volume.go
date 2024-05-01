package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
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

	InitialHostID string `bson:"initial_host_id"`

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

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	if err := j.volume.SetMigrating(true); err != nil {
		j.AddRetryableError(err)
		return
	}

	if !(j.initialHost.Status == evergreen.HostStopped || j.initialHost.Status == evergreen.HostTerminated) {
		j.stopInitialHost(ctx)
		if j.HasErrors() {
			return
		}
	}

	mgrOpts, err := cloud.GetManagerOptions(j.initialHost.Distro)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager options for host '%s'", j.initialHost.Id))
		return
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager for host '%s'", j.initialHost.Id))
		return
	}

	if j.volume.Host != "" {
		// Unmount volume from initial host
		if err := j.initialHost.UnsetHomeVolume(ctx); err != nil {
			j.AddError(errors.Wrapf(err, "unsetting home volume '%s' from host '%s'", j.VolumeID, j.InitialHostID))
			return
		}

		if err := mgr.DetachVolume(ctx, j.initialHost, j.VolumeID); err != nil {
			j.AddError(errors.Wrapf(err, "detaching volume '%s'", j.VolumeID))
			return
		}

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
	newHost, err := host.FindUpHostWithHomeVolume(ctx, j.VolumeID)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "finding host with volume '%s'", j.VolumeID))
		return
	}
	if newHost == nil {
		j.startNewHost(ctx)
		if j.HasErrors() {
			return
		}
	}

	// If not terminated, set initial host to have expiration in 24 hours
	if j.initialHost.Status == evergreen.HostStopped {
		err := j.initialHost.SetExpirationTime(ctx, time.Now().Add(evergreen.DefaultSpawnHostExpiration))
		if err != nil {
			j.AddError(errors.Wrapf(err, "setting expiration for host '%s'", j.InitialHostID))
			return
		}

	}
}

// stopInitialHost inspects the initial host's status and stops it if the host is still running.
func (j *volumeMigrationJob) stopInitialHost(ctx context.Context) {
	ts := utility.RoundPartOfMinute(1).Format(TSFormat)
	stopJob := NewSpawnhostStopJob(j.initialHost, false, evergreen.ModifySpawnHostManual, evergreen.User, ts)
	err := amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), stopJob)
	if err != nil {
		j.AddRetryableError(err)
		return
	}
	grip.Info(message.Fields{
		"message":         "stopping initial host",
		"job_id":          j.ID(),
		"initial_host_id": j.InitialHostID,
	})
	j.AddRetryableError(errors.Errorf("initial host '%s' not yet stopped", j.InitialHostID))
}

// startNewHost attempts to start a new host with the volume attached.
func (j *volumeMigrationJob) startNewHost(ctx context.Context) {
	// Ensure volume has been detached
	if j.volume.Host != "" {
		j.AddRetryableError(errors.New("volume still attached to host"))
		return
	}

	j.ModifyOptions.HomeVolumeID = j.VolumeID
	intentHost, err := cloud.CreateSpawnHost(ctx, j.ModifyOptions, j.env.Settings())
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "creating new intent host"))
		return
	}

	if err := intentHost.Insert(ctx); err != nil {
		j.AddError(errors.Wrap(err, "inserting new intent host"))
		return
	}
	event.LogHostCreated(intentHost.Id)
	grip.Info(message.Fields{
		"message":        "new intent host created",
		"job_id":         j.ID(),
		"intent_host_id": intentHost.Id,
		"host_tag":       intentHost.Tag,
		"distro":         intentHost.Distro.Id,
	})
}

// finishJob marks the job as completed and attempts some additional cleanup if this is the job's final attempt.
func (j *volumeMigrationJob) finishJob(ctx context.Context) {
	if !j.RetryInfo().ShouldRetry() || j.RetryInfo().GetRemainingAttempts() == 0 {
		volumeHost, err := host.FindUpHostWithHomeVolume(ctx, j.VolumeID)
		if err != nil {
			j.AddRetryableError(errors.Wrapf(err, "finding host with volume '%s'", j.VolumeID))
			return
		}
		if volumeHost == nil || volumeHost.Id == j.InitialHostID {
			event.LogVolumeMigrationFailed(j.InitialHostID, j.Error())
			grip.Error(message.Fields{
				"message": "volume failed to migrate",
				"job_id":  j.ID(),
				"host_id": j.InitialHostID,
			})
		}

		if j.volume != nil {
			err := j.volume.SetMigrating(false)
			if err != nil {
				j.AddRetryableError(err)
				return
			}

		}

	}

	j.MarkComplete()
}

func (j *volumeMigrationJob) populateIfUnset(ctx context.Context) error {
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
		// If volume was initially attached to a now-terminated host, query for this host by its home volume field.
		if j.volume.Host == "" {
			initialHost, err := host.FindLatestTerminatedHostWithHomeVolume(ctx, j.VolumeID, j.ModifyOptions.UserName)
			if err != nil {
				return errors.Wrapf(err, "getting host attached to volume '%s'", j.VolumeID)
			}
			if initialHost == nil {
				return errors.Errorf("host attached to volume '%s' not found", j.VolumeID)
			}
			j.InitialHostID = initialHost.Id
		} else {

			j.InitialHostID = j.volume.Host
		}
	}

	if j.initialHost == nil {
		initialHost, err := host.FindOneId(ctx, j.InitialHostID)
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
