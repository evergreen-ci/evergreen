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
	spawnhostStartName = "spawnhost-start"
)

func init() {
	registry.AddJobType(spawnhostStartName,
		func() amboy.Job { return makeSpawnhostStartJob() })
}

type spawnhostStartJob struct {
	// TODO (EVG-16066): remove HostID and UserID since they are no longer
	// necessary, except for temporary backward compatibility.
	HostID                string `bson:"host_id" json:"host_id" yaml:"host_id"`
	UserID                string `bson:"user_id" json:"user_id" yaml:"user_id"`
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`

	host *host.Host
	env  evergreen.Environment
}

func makeSpawnhostStartJob() *spawnhostStartJob {
	j := &spawnhostStartJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostStartName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnhostStartJob returns a job to start a stopped spawn host.
func NewSpawnhostStartJob(h *host.Host, user, ts string) amboy.Job {
	j := makeSpawnhostStartJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStartName, user, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	return j
}

func (j *spawnhostStartJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// Setting the base fields are for temporary backward compatibility with
	// pending jobs already stored in the DB.
	// TODO (EVG-16066): remove this check once all old versions of the job are
	// complete.
	if j.HostID != "" {
		j.CloudHostModification.HostID = j.HostID
	}
	if j.UserID != "" {
		j.CloudHostModification.UserID = j.UserID
	}

	if err := j.CloudHostModification.modifyHost(ctx, func(mgr cloud.Manager, h *host.Host, user string) error {
		if err := mgr.StartInstance(ctx, h, user); err != nil {
			event.LogHostStartFinished(h.Id, false)
			return errors.Wrapf(err, "starting spawn host '%s'", h.Id)
		}

		event.LogHostStartFinished(h.Id, true)

		return nil
	}); err != nil {
		j.AddError(err)
		return
	}
}
