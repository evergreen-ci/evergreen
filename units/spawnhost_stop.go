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
	"github.com/pkg/errors"
)

const (
	spawnhostStopName = "spawnhost-stop"
)

func init() {
	registry.AddJobType(spawnhostStopName,
		func() amboy.Job { return makeSpawnhostStopJob() })
}

type spawnhostStopJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	UserID   string `bson:"user_id" json:"user_id" yaml:"user_id"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	host *host.Host
	env  evergreen.Environment
}

func makeSpawnhostStopJob() *spawnhostStopJob {
	j := &spawnhostStopJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostStopName,
				Version: 0,
			},
		},
	}
	return j
}

func NewSpawnhostStopJob(h *host.Host, user, ts string) amboy.Job {
	j := makeSpawnhostStopJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStopName, user, h.Id, ts))
	j.HostID = h.Id
	j.UserID = user
	return j
}

func (j *spawnhostStopJob) Run(ctx context.Context) {
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

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		j.AddError(errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id))
		return
	}
	cloudManager, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost stop job"))
		return
	}

	// Stop instance using the cloud manager
	if err := cloudManager.StopInstance(ctx, j.host, j.UserID); err != nil {
		j.AddError(errors.Wrap(err, "error stopping spawnhost using cloud manager"))
		event.LogHostStopFinished(j.host.Id, false)
		return
	}

	event.LogHostStopFinished(j.host.Id, true)
	return
}
