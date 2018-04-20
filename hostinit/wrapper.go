package hostinit

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var retryHostError = errors.New("host status is starting after running provisioning")

func SetupHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"message": "attempting to setup host",
		"distro":  h.Distro.Id,
		"hostid":  h.Id,
		"DNS":     h.Host,
		"runner":  RunnerName,
	})

	// check whether or not the host is ready for its setup script to be run
	// if the host isn't ready (for instance, it might not be up yet), skip it
	if ready, err := IsHostReady(ctx, h, settings); !ready {
		m := message.Fields{
			"message": "host not ready for setup",
			"hostid":  h.Id,
			"DNS":     h.Host,
			"distro":  h.Distro.Id,
			"runner":  RunnerName,
		}

		if err != nil {
			grip.Error(message.WrapError(err, m))
			return err
		}

		grip.Info(m)
	}

	if err := ctx.Err(); err != nil {
		return errors.Wrapf(err, "hostinit canceled during setup for host %s", h.Id)
	}

	setupStartTime := time.Now()
	grip.Info(message.Fields{
		"message": "provisioning host",
		"runner":  RunnerName,
		"distro":  h.Distro.Id,
		"hostid":  h.Id,
		"DNS":     h.Host,
	})

	if err := provisionHost(ctx, h, settings); err != nil {
		event.LogHostProvisionError(h.Id)

		grip.Error(message.WrapError(err, message.Fields{
			"message": "provisioning host encountered error",
			"runner":  RunnerName,
			"distro":  h.Distro.Id,
			"hostid":  h.Id,
		}))

		// notify the admins of the failure
		subject := fmt.Sprintf("%v Evergreen provisioning failure on %v",
			notify.ProvisionFailurePreface, h.Distro.Id)
		hostLink := fmt.Sprintf("%v/host/%v", settings.Ui.Url, h.Id)
		message := fmt.Sprintf("Provisioning failed on %v host -- %v: see %v",
			h.Distro.Id, h.Id, hostLink)

		if err := notify.NotifyAdmins(subject, message, settings); err != nil {
			return errors.Wrap(err, "problem sending host init error email")
		}
	}

	// ProvisionHost allows hosts to fail provisioning a few
	// times during host start up, to account for the fact
	// that hosts often need extra time to come up.
	//
	// In these cases, ProvisionHost returns a nil error but
	// does not change the host status.
	if h.Status == evergreen.HostStarting {
		return errors.Wrapf(retryHostError, "retrying for '%s', after %d attempts",
			h.Id, h.ProvisionAttempts)
	}

	grip.Info(message.Fields{
		"message":  "successfully finished provisioning host",
		"hostid":   h.Id,
		"DNS":      h.Host,
		"distro":   h.Distro.Id,
		"runner":   RunnerName,
		"attempts": h.ProvisionAttempts,
		"runtime":  time.Since(setupStartTime),
	})

	return nil
}
