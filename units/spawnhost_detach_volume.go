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
	spawnhostDetachVolumeName = "spawnhost-detach-volume"
)

func init() {
	registry.AddJobType(spawnhostDetachVolumeName,
		func() amboy.Job { return makeSpawnhostDetachVolumeJob() })
}

type spawnhostDetachVolumeJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	host     *host.Host
	volumeID string
	env      evergreen.Environment
}

func makeSpawnhostDetachVolumeJob() *spawnhostDetachVolumeJob {
	j := &spawnhostDetachVolumeJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostDetachVolumeName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostDetachVolumeJob(h *host.Host, volumeID, ts string) amboy.Job {
	j := makeSpawnhostDetachVolumeJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", spawnhostDetachVolumeName, h.Id, ts))
	j.HostID = h.Id
	j.host = h
	j.volumeID = volumeID
	return j
}

func (j *spawnhostDetachVolumeJob) Run(ctx context.Context) {
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
		"message": "detaching volume from spawnhost",
		"job_id":  j.ID(),
		"host_id": j.HostID,
		"volume":  j.volumeID,
	})

	mgrOpts := cloud.ManagerOpts{
		Provider: j.host.Provider,
		Region:   cloud.GetRegion(j.host.Distro),
	}
	mgr, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost detach volume job"))
		return
	}

	if err = mgr.DetachVolume(ctx, j.host, j.volumeID); err != nil {
		j.AddError(errors.Wrapf(err, "error detaching volume %s from spawnhost %s", j.volumeID, j.HostID))
	}

	return
}
