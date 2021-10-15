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
	spawnhostExpirationCheckName = "spawnhost-expiration-check"
)

func init() {
	registry.AddJobType(spawnhostExpirationCheckName,
		func() amboy.Job { return makeSpawnhostExpirationCheckJob() })
}

type spawnhostExpirationCheckJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	host *host.Host
	env  evergreen.Environment
}

func makeSpawnhostExpirationCheckJob() *spawnhostExpirationCheckJob {
	j := &spawnhostExpirationCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostExpirationCheckName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostExpirationCheckJob(ts string, h *host.Host) amboy.Job {
	j := makeSpawnhostExpirationCheckJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", spawnhostExpirationCheckName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnhostExpirationCheckName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.HostID = h.Id
	return j
}

func (j *spawnhostExpirationCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	var err error

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error getting host '%s' in spawnhost expiration check job", j.HostID))
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
		j.AddError(errors.Wrapf(err, "error getting cloud manager for host '%s' in spawnhost expiration check job", j.HostID))
		return
	}
	noExpiration := true
	if err := cloudManager.ModifyHost(ctx, j.host, host.HostModifyOptions{NoExpiration: &noExpiration}); err != nil {
		j.AddError(errors.Wrapf(err, "error extending expiration for spawn host '%s' using cloud manager", j.HostID))
		return
	}

	return
}
