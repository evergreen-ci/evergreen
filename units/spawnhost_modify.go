package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
)

const (
	spawnHostStatusChangeScopeName = "spawn-host-status-change"

	spawnHostModifyOtelAttributePrefix = "evergreen.spawn_host_modify"
)

// CloudHostModification is a helper to perform cloud manager operations on
// a single host.
type CloudHostModification struct {
	HostID string                          `bson:"host_id" json:"host_id" yaml:"host_id"`
	UserID string                          `bson:"user_id" json:"user_id" yaml:"user_id"`
	Source evergreen.ModifySpawnHostSource `bson:"source" json:"source" yaml:"source"`

	host *host.Host
	env  evergreen.Environment
}

func (m *CloudHostModification) modifyHost(ctx context.Context, op func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error) error {
	if m.env == nil {
		m.env = evergreen.GetEnvironment()
	}

	var err error
	if m.host == nil {
		m.host, err = host.FindOneByIdOrTag(ctx, m.HostID)
		if err != nil {
			return errors.Wrap(err, "finding host")
		}
		if m.host == nil {
			return errors.Errorf("host '%s' not found", m.HostID)
		}
	}

	ctx = utility.ContextWithAttributes(ctx, m.hostAttributes(m.host))

	mgrOpts, err := cloud.GetManagerOptions(m.host.Distro)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager options")
	}
	cloudManager, err := cloud.GetManager(ctx, m.env, mgrOpts)
	if err != nil {
		return errors.Wrap(err, "getting cloud manager")
	}

	return op(ctx, cloudManager, m.host, m.UserID)
}

func (m *CloudHostModification) hostAttributes(h *host.Host) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(evergreen.HostIDOtelAttribute, h.Id),
		attribute.String(evergreen.DistroIDOtelAttribute, h.Distro.Id),
		attribute.String(evergreen.HostStartedByOtelAttribute, h.StartedBy),
		attribute.Bool(evergreen.HostNoExpirationOtelAttribute, h.NoExpiration),
		attribute.String(evergreen.HostInstanceTypeOtelAttribute, h.InstanceType),
		attribute.String(fmt.Sprintf("%s.source", spawnHostModifyOtelAttributePrefix), string(m.Source)),
	}
}

const (
	spawnhostModifyName = "spawnhost-modify"
)

func init() {
	registry.AddJobType(spawnhostModifyName, func() amboy.Job {
		return makeSpawnhostModifyJob()
	})
}

type spawnhostModifyJob struct {
	ModifyOptions         host.HostModifyOptions `bson:"modify_options" json:"modify_options" yaml:"modify_options"`
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
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

	modifyCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		if err := mgr.ModifyHost(ctx, h, j.ModifyOptions); err != nil {
			event.LogHostModifyError(h.Id, err.Error())
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error modifying spawn host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
				"distro":   h.Distro.Id,
				"options":  j.ModifyOptions,
			}))
			return errors.Wrap(err, "modifying spawn host")
		}

		event.LogHostModifySucceeded(h.Id)
		grip.Info(message.Fields{
			"message":  "modified spawn host",
			"host_id":  h.Id,
			"host_tag": h.Tag,
			"distro":   h.Distro.Id,
			"options":  j.ModifyOptions,
		})
		return nil
	}
	if err := j.CloudHostModification.modifyHost(ctx, modifyCloudHost); err != nil {
		j.AddError(err)
		return
	}
}
