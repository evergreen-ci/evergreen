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
	spawnhostModifyName = "spawnhost-modify"
)

func init() {
	registry.AddJobType(spawnhostModifyName,
		func() amboy.Job { return makeSpawnhostModifyJob() })
}

type spawnhostModifyJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	host     *host.Host
	env      evergreen.Environment

	changes host.HostModifyOptions
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
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostModifyJob(h *host.Host, changes host.HostModifyOptions) amboy.Job {
	j := makeSpawnhostModifyJob()
	j.SetID(fmt.Sprintf("%s.%s", spawnhostModifyName, h.Id))
	j.changes = changes
	return j
}

func (j *spawnhostModifyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: j.host.Provider,
		Region:   cloud.GetRegion(j.host.Distro),
	}
	cloudManager, err := cloud.GetManager(ctx, mgrOpts, j.env.Settings())
	if err != nil {
		j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost modify job"))
		return
	}

	// Modify spawnhost using the cloud manager
	if err := cloudManager.ModifyHost(ctx, j.host, j.changes); err != nil {
		j.AddError(errors.Wrap(err, "error modifying spawnhost using cloud manager"))
		return
	}

	// Push changes to the database
	if err := j.host.ModifySpawnHost(j.changes); err != nil {
		j.AddError(errors.Wrap(err, "error updating spawnhost in database after modify"))
		return
	}

	return
}
