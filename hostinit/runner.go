package hostinit

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

// Runner executes the hostinit process.
type Runner struct{}

const (
	RunnerName = "hostinit"
)

// host init "starts and initializes new Evergreen hosts"
func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if flags.HostinitDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "hostinit is disabled, exiting",
		})
		return nil
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	// starting hosts and provisioning hosts don't need to run serially since
	// the hosts that were just started aren't immediately ready for provisioning
	catcher := grip.NewSimpleCatcher()
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := message.Fields{
			"runner": RunnerName,
			"method": "startHosts",
		}

		var hadErrors bool
		if err := startHosts(ctx, config); err != nil {
			err = errors.Wrap(err, "Error starting hosts")
			catcher.Add(err)
			hadErrors = true
			msg["error"] = err.Error()
			msg["status"] = "failed"
		} else {
			msg["status"] = "success"
		}

		msg["runtime"] = time.Since(startTime)
		msg["span"] = time.Since(startTime).String()

		grip.ErrorWhen(hadErrors, msg)
		grip.InfoWhen(!hadErrors, msg)

	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		msg := message.Fields{
			"runner": RunnerName,
			"method": "setupReadyHosts",
		}

		var hadErrors bool
		if err := setupReadyHosts(ctx, config); err != nil {
			err = errors.Wrap(err, "Error provisioning hosts")
			catcher.Add(err)
			hadErrors = true
			msg["error"] = err.Error()
			msg["status"] = "failed"
		} else {
			msg["status"] = "success"
		}

		msg["runtime"] = time.Since(startTime)
		msg["span"] = time.Since(startTime).String()

		grip.ErrorWhen(hadErrors, msg)
		grip.InfoWhen(!hadErrors, msg)
	}()

	wg.Wait()

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem updating process status",
			"runner":  RunnerName,
		}))
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": time.Since(startTime),
		"span":    time.Since(startTime).String(),
		"status":  "success",
	})

	return nil
}
