package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	volumeDeletionName = "volume-deletion"
)

func init() {
	registry.AddJobType(volumeDeletionName,
		func() amboy.Job { return makeVolumeDeletionJob() })
}

type volumeDeletionJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	VolumeID string `bson:"volume_id" yaml:"volume_id"`
	Provider string `bson:"provider" yaml:"provider"`

	volume *host.Volume
	env    evergreen.Environment
}

func makeVolumeDeletionJob() *volumeDeletionJob {
	j := &volumeDeletionJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeDeletionName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewVolumeDeletionJob(ts string, v *host.Volume) amboy.Job {
	j := makeVolumeDeletionJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", volumeDeletionName, v.ID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", volumeDeletionName, v.ID)})
	j.SetEnqueueAllScopes(true)
	j.VolumeID = v.ID
	j.Provider = evergreen.ProviderNameEc2OnDemand
	return j
}

func (j *volumeDeletionJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	var err error

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.volume == nil {
		j.volume, err = host.FindVolumeByID(j.VolumeID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error getting volume '%s'", j.VolumeID))
			return
		}
		if j.volume == nil {
			j.AddError(errors.Errorf("no volume '%s' exists", j.VolumeID))
			return
		}
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: j.Provider,
		Region:   cloud.AztoRegion(j.volume.AvailabilityZone),
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't get manager for volume '%s'", j.VolumeID))
		return
	}

	if err := mgr.DeleteVolume(ctx, j.volume); err != nil {
		j.AddError(errors.Wrapf(err, "can't delete volume '%s'", j.VolumeID))
		return
	}

	return
}
