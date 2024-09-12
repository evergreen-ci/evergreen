package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
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
				catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, &containers[i], true, reason), "disabling poisoned container '%s' under parent '%s'", containers[i].Id, h.ParentID)
			}
			catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, parent, true, reason), "disabling poisoned parent '%s' of container '%s'", h.ParentID, h.Id)
		}
	} else {
		catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, h, true, reason), "disabling poisoned host '%s'", h.Id)
	}

	return catcher.Resolve()
}

// DisableAndNotifyPoisonedHost disables an unhealthy host so that it cannot run
// any more tasks, clears any tasks that have been stranded on it, and enqueues
// a job to notify that a host was disabled. If canDecommission is true and the
// host is an ephemeral host, it will decommission the host instead of
// quarantine it.
func DisableAndNotifyPoisonedHost(ctx context.Context, env evergreen.Environment, h *host.Host, canDecommission bool, reason string) error {
	if utility.StringSliceContains(evergreen.DownHostStatus, h.Status) {
		return nil
	}

	if canDecommission && h.Provider != evergreen.ProviderNameStatic {
		if err := h.SetDecommissioned(ctx, evergreen.User, true, reason); err != nil {
			return errors.Wrapf(err, "decommissioning host '%s'", h.Id)
		}
	} else {
		if err := h.SetQuarantined(ctx, evergreen.User, reason); err != nil {
			return errors.Wrapf(err, "quarantining host '%s'", h.Id)
		}
	}

	if err := model.ClearAndResetStrandedHostTask(ctx, env.Settings(), h); err != nil {
		return errors.Wrap(err, "clearing stranded task from host")
	}

	return errors.Wrapf(amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewDecoHostNotifyJob(env, h, nil, reason)), "enqueueing decohost notify job for host '%s'", h.Id)
}

// EnqueueHostReprovisioningJob enqueues a job to reprovision a host. For hosts
// that do not need to reprovision, this is a no-op.
func EnqueueHostReprovisioningJob(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ts := utility.RoundPartOfMinute(0).Format(TSFormat)

	switch h.NeedsReprovision {
	case host.ReprovisionToLegacy:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewConvertHostToLegacyProvisioningJob(env, *h, ts)); err != nil {
			return errors.Wrap(err, "enqueueing job to reprovision host to legacy")
		}
	case host.ReprovisionToNew:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewConvertHostToNewProvisioningJob(env, *h, ts)); err != nil {
			return errors.Wrap(err, "enqueueing job to reprovision host to new")
		}
	case host.ReprovisionRestartJasper:
		if err := amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewRestartJasperJob(env, *h, ts)); err != nil {
			return errors.Wrap(err, "enqueueing jobs to restart Jasper")
		}
	}

	return nil
}

// EnqueueSpawnHostModificationJob enqueues a job to modify a spawn host.
func EnqueueSpawnHostModificationJob(ctx context.Context, env evergreen.Environment, j amboy.Job) error {
	queueCtx, _ := env.Context()
	q, err := env.RemoteQueueGroup().Get(queueCtx, spawnHostModificationQueueGroup)
	if err != nil {
		return errors.Wrap(err, "getting spawn host modification queue")
	}
	return amboy.EnqueueUniqueJob(ctx, q, j)
}

// EnqueueTerminateHostJob enqueues a job to terminate a host.
func EnqueueTerminateHostJob(ctx context.Context, env evergreen.Environment, j amboy.Job) error {
	queueCtx, _ := env.Context()
	q, err := env.RemoteQueueGroup().Get(queueCtx, terminateHostQueueGroup)
	if err != nil {
		return errors.Wrap(err, "getting host termination queue")
	}
	return amboy.EnqueueUniqueJob(ctx, q, j)
}
