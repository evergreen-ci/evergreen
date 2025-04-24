package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
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
		j.host, err = host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
			return
		}
	}

	mgrOpts, err := cloud.GetManagerOptions(j.host.Distro)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager options for host '%s'", j.host.Id))
		return
	}
	cloudManager, err := cloud.GetManager(ctx, j.env, mgrOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud manager for host '%s'", j.HostID))
		return
	}
	if err := cloudManager.ModifyHost(ctx, j.host, host.HostModifyOptions{NoExpiration: utility.TruePtr()}); err != nil {
		j.AddError(errors.Wrapf(err, "extending expiration for host '%s'", j.HostID))
		return
	}
	// If an unexpirable host hasn't been used in a while, send an email encouraging the user to remove the host.
	j.AddError(tryIdleSpawnHostNotification(ctx, j.host))

}

// tryIdleSpawnHostNotification attempts to insert a subscription and notification for this spawn host.
func tryIdleSpawnHostNotification(ctx context.Context, h *host.Host) error {
	shouldNotify, err := h.ShouldNotifyStoppedSpawnHostIdle(ctx)
	if err != nil || !shouldNotify {
		return err
	}
	usr, err := user.FindOneByIdContext(ctx, h.StartedBy)
	if err != nil {
		return errors.Wrapf(err, "finding user '%s'", h.StartedBy)
	}
	if usr == nil {
		return errors.Errorf("user '%s' not found", h.StartedBy)
	}
	subscriber := event.NewEmailSubscriber(usr.Email())
	subscription := event.NewSpawnHostIdleWarningSubscription(h.Id, subscriber)
	if err = subscription.Upsert(ctx); err != nil {
		return errors.Wrap(err, "upserting idle spawn host subscription")
	}

	event.LogSpawnHostIdleNotification(ctx, h.Id)
	return nil
}
