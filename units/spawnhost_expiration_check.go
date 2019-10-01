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

func NewSpawnhostExpirationCheckJob(ts string) amboy.Job {
	j := makeSpawnhostExpirationCheckJob()
	j.SetID(fmt.Sprintf("%s.%s", spawnhostExpirationCheckName, ts))
	return j
}

func (j *spawnhostExpirationCheckJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	hostsToExtend, err := host.FindHostsWithNoExpirationToExtend()
	if err != nil {
		j.AddError(err)
		return
	}

	hostsByManager := cloud.GroupHostsByManager(hostsToExtend)
	for mgrOpts, hosts := range hostsByManager {
		cloudManager, err := cloud.GetManager(ctx, mgrOpts, j.env.Settings())
		if err != nil {
			j.AddError(errors.Wrap(err, "error getting cloud manager for spawnhost expiration check job"))
			return
		}
		for _, h := range hosts {
			noExpiration := true
			if err := cloudManager.ModifyHost(ctx, &h, host.HostModifyOptions{NoExpiration: &noExpiration}); err != nil {
				j.AddError(errors.Wrap(err, "error extending spawnhost expiration using cloud manager"))
				return
			}
		}

	}

	return
}
