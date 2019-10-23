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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	spawnhostAttachVolumeName = "spawnhost-attach-volume"
)

func init() {
	registry.AddJobType(spawnhostAttachVolumeName,
		func() amboy.Job { return makeSpawnhostAttachVolumeJob() })
}

type spawnhostAttachVolumeJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	host       *host.Host
	attachment host.VolumeAttachment
	env        evergreen.Environment
}

func makeSpawnhostAttachVolumeJob() *spawnhostAttachVolumeJob {
	j := &spawnhostAttachVolumeJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostAttachVolumeName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostAttachVolumeJob(h *host.Host, attachment host.VolumeAttachment, ts string) amboy.Job {
	j := makeSpawnhostAttachVolumeJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", spawnhostAttachVolumeName, h.Id, ts))
	j.host = h
	j.HostID = h.Id
	j.attachment = attachment
	return j
}

func (j *spawnhostAttachVolumeJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	var err error
	if j.host == nil {
		j.host, err = host.FindOneByIdOrTag(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.ID()))
			return
		}
	}

	grip.Info(message.Fields{
		"message": "attaching volume to spawnhost",
		"job_id":  j.ID(),
		"host_id": j.HostID,
		"volume":  j.attachment,
	})

	mgrOpts := cloud.ManagerOpts{
		Provider: j.host.Provider,
		Region:   cloud.GetRegion(j.host.Distro),
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost attach volume job"))
		return
	}

	if err = mgr.AttachVolume(ctx, j.host, j.attachment); err != nil {
		j.AddError(errors.Wrapf(err, "error attaching volume %s for spawnhost %s", j.attachment.VolumeID, j.HostID))
	}

	return
}
