package units

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func HandlePoisonedHost(ctx context.Context, env evergreen.Environment, h *host.Host, reason string) error {
	if h == nil {
		return errors.New("no host found")
	}
	catcher := grip.NewBasicCatcher()
	if h.ParentID != "" {
		parent, err := host.FindOneId(h.ParentID)
		if err != nil {
			return errors.Wrap(err, "error finding parent host")
		}
		if parent != nil {
			containers, err := parent.GetActiveContainers()
			if err != nil {
				return errors.Wrap(err, "error getting containers")
			}

			for _, container := range containers {
				catcher.Add(DisableAndNotifyPoisonedHost(ctx, env, container, reason))
			}
			catcher.Add(DisableAndNotifyPoisonedHost(ctx, env, *parent, reason))
		}
	}
	catcher.Add(DisableAndNotifyPoisonedHost(ctx, env, *h, reason))
	return catcher.Resolve()
}

func DisableAndNotifyPoisonedHost(ctx context.Context, env evergreen.Environment, h host.Host, reason string) error {
	if h.Status == evergreen.HostDecommissioned || h.Status == evergreen.HostTerminated {
		return nil
	}
	err := h.DisablePoisonedHost(reason)
	if err != nil {
		return errors.Wrap(err, "error disabling poisoned host")
	}
	return env.RemoteQueue().Put(ctx, NewDecoHostNotifyJob(env, &h, nil, reason))
}
