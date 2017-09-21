package hostinit

import (
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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

	init := &HostInit{
		Settings: config,
		GUID:     util.RandomString(),
	}

	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.HostinitDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "hostinit is disabled, exiting",
			"GUID":    init.GUID,
		})
		return nil
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
		"GUID":    init.GUID,
	})

	// starting hosts and provisioning hosts don't need to run serially since
	// the hosts that were just started aren't immediately ready for provisioning
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := init.startHosts(ctx); err != nil {
			err = errors.Wrap(err, "Error starting hosts")
			grip.Error(message.Fields{
				"GUID":    init.GUID,
				"runner":  RunnerName,
				"error":   err.Error(),
				"status":  "failed",
				"method":  "startHosts",
				"runtime": time.Since(startTime),
				"span":    time.Since(startTime).String(),
			})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := init.setupReadyHosts(ctx); err != nil {
			err = errors.Wrap(err, "Error provisioning hosts")
			grip.Error(message.Fields{
				"GUID":    init.GUID,
				"runner":  RunnerName,
				"error":   err.Error(),
				"status":  "failed",
				"method":  "setupReadyHosts",
				"runtime": time.Since(startTime),
			})
		}
	}()

	wg.Wait()

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(errors.Wrap(err, "problem updating process status"))
	}

	grip.Info(message.Fields{
		"GUID":    init.GUID,
		"runner":  RunnerName,
		"runtime": time.Since(startTime),
		"span":    time.Since(startTime).String(),
		"status":  "success",
	})

	return nil
}
