package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
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

func runSpawnHostSetupScript(ctx context.Context, env evergreen.Environment, h *host.Host) error {
	script := fmt.Sprintf("cd /home/%s\n%s", h.User, h.ProvisionOptions.SetupScript)
	ts := utility.RoundPartOfMinute(0).Format(TSFormat)
	j := NewHostExecuteJob(env, *h, script, false, "", ts)
	j.Run(ctx)

	return errors.Wrapf(j.Error(), "error running setup script for spawn host")
}
