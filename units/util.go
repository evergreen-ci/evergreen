package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func HandlePoisonedHost(ctx context.Context, env evergreen.Environment, h *host.Host, reason string) error {
	if h == nil {
		return errors.New("host cannot be nil")
	}
	catcher := grip.NewBasicCatcher()
	if h.ParentID != "" {
		parent, err := host.FindOneId(ctx, h.ParentID)
		if err != nil {
			return errors.Wrapf(err, "finding parent host for container '%s'", h.Id)
		}
		if parent != nil {
			containers, err := parent.GetActiveContainers(ctx)
			if err != nil {
				return errors.Wrapf(err, "getting containers under parent container '%s'", h.ParentID)
			}

			for i := range containers {
				catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, &containers[i], reason), "disabling poisoned container '%s' under parent '%s'", containers[i].Id, h.ParentID)
			}
			catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, parent, reason), "disabling poisoned parent '%s' of container '%s'", h.ParentID, h.Id)
		}
	} else {
		catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, h, reason), "disabling poisoned host '%s'", h.Id)
	}

	return catcher.Resolve()
}

func DisableAndNotifyPoisonedHost(ctx context.Context, env evergreen.Environment, h *host.Host, reason string) error {
	if utility.StringSliceContains(evergreen.DownHostStatus, h.Status) {
		return nil
	}

	err := h.DisablePoisonedHost(ctx, reason)
	if err != nil {
		return errors.Wrap(err, "disabling poisoned host")
	}

	hostUptime := time.Since(h.CreationTime)
	grip.Error(message.Fields{
		"operation":   "disabling poisoned host",
		"message":     reason,
		"distro":      h.Distro.Id,
		"provider":    h.Provider,
		"host":        h.Id,
		"uptime_secs": hostUptime.Seconds(),
		"uptime_span": hostUptime.String(),
		"host_id":     h.Id,
		"target":      fmt.Sprintf("%s@%s", h.Distro.User, h.Host),
	})

	return model.ClearAndResetStrandedHostTask(ctx, env.Settings(), h)
}

// EnqueueHostReprovisioningJob enqueues a job to reprovision a host. For hosts
// that do not need to reprovision, this is a no-op.
func EnqueueHostReprovisioningJob(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ts := utility.RoundPartOfHour(15).Format(TSFormat)

	switch h.NeedsReprovision {
	case host.ReprovisionToLegacy:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewConvertHostToLegacyProvisioningJob(env, *h, ts, 0)); err != nil {
			return errors.Wrap(err, "enqueueing job to reprovision host to legacy")
		}
	case host.ReprovisionToNew:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewConvertHostToNewProvisioningJob(env, *h, ts, 0)); err != nil {
			return errors.Wrap(err, "enqueueing job to reprovision host to new")
		}
	case host.ReprovisionRestartJasper:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewRestartJasperJob(env, *h, ts)); err != nil {
			return errors.Wrap(err, "enqueueing jobs to restart Jasper")
		}
	}

	return nil
}
