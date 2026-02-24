package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const spawnHostTerminationJobName = "spawnhost-termination"

func init() {
	registry.AddJobType(spawnHostTerminationJobName, func() amboy.Job {
		return makeSpawnHostTerminationJob()
	})
}

type spawnHostTerminationJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnHostTerminationJob() *spawnHostTerminationJob {
	j := &spawnHostTerminationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnHostTerminationJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnHostTerminationJob returns a job to terminate a spawn host
// with the given source indicating why it is being terminated.
func NewSpawnHostTerminationJob(h *host.Host, user, ts string, source evergreen.ModifySpawnHostSource) amboy.Job {
	j := makeSpawnHostTerminationJob()
	j.SetID(fmt.Sprintf("%s.%s.%s", spawnHostTerminationJobName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	j.CloudHostModification.Source = source
	return j
}

func (j *spawnHostTerminationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	terminateCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		reason := "user requested spawn host termination"
		if j.CloudHostModification.Source == evergreen.ModifySpawnHostProjectSettings {
			reason = "project disabled debug spawn hosts"
		}
		if err := mgr.TerminateInstance(ctx, h, user, reason); err != nil {
			return err
		}

		return nil
	}
	if err := j.CloudHostModification.modifyHost(ctx, terminateCloudHost); err != nil {
		j.AddError(err)
		return
	}
}
