package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	spawnhostModifyName = "spawnhost-modify"
)

func init() {
	registry.AddJobType(spawnhostModifyName,
		func() amboy.Job { return makeSpawnhostModifyJob() })
}

type spawnhostModifyJob struct {
	HostID        string                 `bson:"host_id" json:"host_id" yaml:"host_id"`
	ModifyOptions host.HostModifyOptions `bson:"modify_options" json:"modify_options" yaml:"modify_options"`
	job.Base      `bson:"job_base" json:"job_base" yaml:"job_base"`

	host *host.Host
	env  evergreen.Environment
}

func makeSpawnhostModifyJob() *spawnhostModifyJob {
	j := &spawnhostModifyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostModifyName,
				Version: 0,
			},
		},
	}
	return j
}

func NewSpawnhostModifyJob(h *host.Host, changes host.HostModifyOptions, ts string) amboy.Job {
	j := makeSpawnhostModifyJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", spawnhostModifyName, h.Id, ts))
	j.HostID = h.Id
	j.ModifyOptions = changes
	return j
}

func (j *spawnhostModifyJob) Run(ctx context.Context) {
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
		"message": "modifying spawnhost",
		"job_id":  j.ID(),
		"host_id": j.HostID,
		"changes": j.ModifyOptions,
	})

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id))
		return
	}
	cloudManager, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost modify job"))
		return
	}

	// Modify spawnhost using the cloud manager
	if err := cloudManager.ModifyHost(ctx, j.host, j.ModifyOptions); err != nil {
		j.AddError(errors.Wrap(err, "error modifying spawnhost using cloud manager"))
		event.LogHostModifyFinished(j.host.Id, false)
		return
	}

	event.LogHostModifyFinished(j.host.Id, true)
	return
}
