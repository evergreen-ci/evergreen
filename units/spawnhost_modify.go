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
	spawnHostStatusChangeScopeName = "spawn-host-status-change"
)

// CloudHostModification is a helper to perform cloud manager operations on
// a single host.
type CloudHostModification struct {
	HostID string `bson:"host_id" json:"host_id" yaml:"host_id"`
	UserID string `bson:"user_id" json:"user_id" yaml:"user_id"`

	host *host.Host
	env  evergreen.Environment
}

func (m *CloudHostModification) modifyHost(ctx context.Context, op func(mgr cloud.Manager, h *host.Host, user string) error) error {
	if m.env == nil {
		m.env = evergreen.GetEnvironment()
	}

	var err error
	if m.host == nil {
		m.host, err = host.FindOneByIdOrTag(m.HostID)
		if err != nil {
			return errors.Wrap(err, "finding host")
		}
		if m.host == nil {
			return errors.Wrap(err, "host not found")
		}
	}

	mgrOpts, err := cloud.GetManagerOptions(m.host.Distro)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager options")
	}
	cloudManager, err := cloud.GetManager(ctx, m.env, mgrOpts)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager")
	}

	return op(cloudManager, m.host, m.UserID)
}

const (
	spawnhostModifyName = "spawnhost-modify"
)

func init() {
	registry.AddJobType(spawnhostModifyName,
		func() amboy.Job { return makeSpawnhostModifyJob() })
}

type spawnhostModifyJob struct {
	// TODO (EVG-16066): remove HostID.
	HostID                string                 `bson:"host_id" json:"host_id" yaml:"host_id"`
	ModifyOptions         host.HostModifyOptions `bson:"modify_options" json:"modify_options" yaml:"modify_options"`
	CloudHostModification `bson:"base_spawn_host_modification" json:"base_spawn_host_modification" yaml:"base_spawn_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
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
	j.ModifyOptions = changes
	j.CloudHostModification.HostID = h.Id
	return j
}

func (j *spawnhostModifyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// Setting the base field is for temporary backward compatibility with
	// pending jobs already stored in the DB.
	// TODO (EVG-16066): remove this check once all old versions of the job are
	// complete.
	if j.HostID != "" {
		j.CloudHostModification.HostID = j.HostID
	}

	if err := j.CloudHostModification.modifyHost(ctx, func(mgr cloud.Manager, h *host.Host, user string) error {
		if err := mgr.ModifyHost(ctx, h, j.ModifyOptions); err != nil {
			event.LogHostModifyFinished(h.Id, false)
			return errors.Wrapf(err, "modifying spawn host '%s'", h.Id)
		}

		event.LogHostModifyFinished(h.Id, true)
		return nil
	}); err != nil {
		j.AddError(err)
		return
	}
}
