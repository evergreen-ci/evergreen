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
	j.SetDependency(dependency.NewAlways())
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

	mgrOpts := cloud.ManagerOpts{
		Provider: j.host.Provider,
		Region:   cloud.GetRegion(j.host.Distro),
	}
	cloudManager, err := cloud.GetManager(ctx, mgrOpts, j.env.Settings())
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost stop job"))
		return
	}

	// Stop instance using the cloud manager
	if err := cloudManager.StopInstance(ctx, j.host, j.UserID); err != nil {
		j.AddError(errors.Wrap(err, "error stopping spawnhost using cloud manager"))
		return
	}

	return
}
