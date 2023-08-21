package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	volumeUnstickName = "volume-unstick"
)

func init() {
	registry.AddJobType(volumeUnstickName,
		func() amboy.Job { return makeVolumeUnstickJob() })
}

type volumeUnstickJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	VolumeID string `bson:"volume_id" yaml:"volume_id"`

	env evergreen.Environment
}

func makeVolumeUnstickJob() *volumeUnstickJob {
	j := &volumeUnstickJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    volumeUnstickName,
				Version: 0,
			},
		},
	}
	return j
}

func NewVolumeUnstickJob(ts string, v *host.Volume) amboy.Job {
	j := makeVolumeUnstickJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", volumeUnstickName, v.ID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", volumeUnstickName, v.ID)})
	j.SetEnqueueAllScopes(true)
	j.VolumeID = v.ID
	return j
}

func (j *volumeUnstickJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if err := host.UnsetVolumeHost(j.VolumeID); err != nil {
		j.AddError(errors.Wrapf(err, "unsetting terminated host for volume '%s'", j.VolumeID))
		return
	}
	grip.Info(message.Fields{
		"message":   "unset terminated host for volume",
		"volume_id": j.VolumeID,
		"job_id":    j.ID(),
	})
}
