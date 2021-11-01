package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	volumeExpirationCheckName = "volume-expiration-check"
)

func init() {
	registry.AddJobType(volumeExpirationCheckName,
		func() amboy.Job { return makeVolumeExpirationCheckJob() })
}

type volumeExpirationCheckJob struct {
	job.Base
	VolumeID string `bson:"volume_id" json:"volume_id" yaml:"volume_id"`
	Provider string `bson:"provider" json:"provider" yaml:"provider"`

	volume *host.Volume
	env    evergreen.Environment
}

func makeVolumeExpirationCheckJob() *volumeExpirationCheckJob {
	j := &volumeExpirationCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeExpirationCheckName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewVolumeExpirationCheckJob(ts string, v *host.Volume, provider string) amboy.Job {
	j := makeVolumeExpirationCheckJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", volumeExpirationCheckName, v.ID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", volumeExpirationCheckName, v.ID)})
	j.SetEnqueueAllScopes(true)
	j.VolumeID = v.ID
	j.Provider = provider

	j.volume = v
	return j
}

func (j *volumeExpirationCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	var err error

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.volume == nil {
		j.volume, err = host.FindVolumeByID(j.VolumeID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error getting volume '%s' in volume expiration check job", j.VolumeID))
			return
		}
		if j.volume == nil {
			j.AddError(errors.Wrapf(err, "volume '%s' is not found", j.VolumeID))
			return
		}
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: j.Provider,
		Region:   cloud.AztoRegion(j.volume.AvailabilityZone),
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud manager for volume '%s' in volume expiration check job", j.VolumeID))
		return
	}
	if err := mgr.ModifyVolume(ctx, j.volume, &model.VolumeModifyOptions{NoExpiration: true}); err != nil {
		j.AddError(errors.Wrapf(err, "error extending expiration for volume '%s' using cloud manager", j.VolumeID))
		return
	}

	return
}
