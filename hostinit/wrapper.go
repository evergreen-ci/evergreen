package hostinit

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var (
	errRetryHost           = errors.New("host status is starting after running provisioning")
	errIgnorableCreateHost = errors.New("create host encountered internal error")
)

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
	if ready, err := isHostReady(ctx, h, settings); !ready {
		m := message.Fields{
			"message": "host not ready for setup",
			"hostid":  h.Id,
			"DNS":     h.Host,
			"distro":  h.Distro.Id,
			"runner":  RunnerName,
		}

		if err != nil {
			grip.Error(message.WrapError(err, m))
			return errors.Wrap(errRetryHost, err.Error())
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
		return errors.Wrapf(errRetryHost, "retrying for '%s', after %d attempts",
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

func CreateHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	hostStartTime := time.Now()
	grip.Info(message.Fields{
		"message": "attempting to start host",
		"hostid":  h.Id,
		"runner":  RunnerName,
	})

	cloudManager, err := cloud.GetCloudManager(ctx, h.Provider, settings)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "problem getting cloud provider for host",
			"runner":  RunnerName,
			"host":    h.Id,
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", h.Id, err.Error())
	}

	if err = h.Remove(); err != nil {
		grip.Notice(message.WrapError(err, message.Fields{
			"message": "problem removing intent host",
			"runner":  RunnerName,
			"host":    h.Id,
		}))
		return errors.Wrapf(errIgnorableCreateHost, "problem getting cloud provider for host '%s' [%s]", h.Id, err.Error())
	}

	_, err = cloudManager.SpawnHost(ctx, h)
	if err != nil {
		// we should maybe try and continue-on-error
		// here, if we get many errors, but the chance
		// is that if one fails, the chances of others
		// failing is quite high (at least while all
		// cloud providers are typically the same
		// service provider.)
		return errors.Wrapf(err, "error spawning host %s", h.Id)
	}

	h.Status = evergreen.HostStarting
	h.StartTime = time.Now()

	_, err = h.Upsert()
	if err != nil {
		return errors.Wrapf(err, "error updating host %v", h.Id)
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"message": "successfully started host",
		"hostid":  h.Id,
		"DNS":     h.Host,
		"runtime": time.Since(hostStartTime),
	})

	return nil
}
