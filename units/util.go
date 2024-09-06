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
				catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, &containers[i], reason), "disabling poisoned container '%s' under parent '%s'", containers[i].Id, h.ParentID)
			}
			catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, parent, reason), "disabling poisoned parent '%s' of container '%s'", h.ParentID, h.Id)
		}
	} else {
		catcher.Wrapf(DisableAndNotifyPoisonedHost(ctx, env, h, reason), "disabling poisoned host '%s'", h.Id)
	}

	return catcher.Resolve()
}

// kim: NOTE: make sure this is called whenever quarantining host.
// kim: NOTE: if setting static host to quarantined, need to ensure
// current task is also cleared/reset using
// ClearAndResetStrandedHostTask before clearing the state. That ought
// to restart the task group from scratch.
// kim: NOTE: on top of fixing stranded current task, need to make sure
// single host task group restarts from scratch if it's already pinned
// to this host (and isn't currently running a task in the group).
// May help to create a quarantine helper function to fix up
// prev/current task state and restart as needed. If the prev/current
// task state is fixed there, then this logic is not needed.
func DisableAndNotifyPoisonedHost(ctx context.Context, env evergreen.Environment, h *host.Host, reason string) error {
	if utility.StringSliceContains(evergreen.DownHostStatus, h.Status) {
		return nil
	}

	err := h.DisablePoisonedHost(ctx, reason)
	if err != nil {
		return errors.Wrap(err, "disabling poisoned host")
	}

	if err = amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewDecoHostNotifyJob(env, h, nil, reason)); err != nil {
		return errors.Wrap(err, "enqueueing decohost notify job")
	}

	// kim: TODO: additionally need to clear previous task if possible for
	// single host task groups. It's possible for only previous task to be set
	// and current task may not be set, even though task group is still in
	// progress.
	return model.ClearAndResetStrandedHostTaskOrTaskGroup(ctx, env.Settings(), h)
}

// EnqueueHostReprovisioningJob enqueues a job to reprovision a host. For hosts
// that do not need to reprovision, this is a no-op.
func EnqueueHostReprovisioningJob(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ts := utility.RoundPartOfMinute(0).Format(TSFormat)

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
