package hostinit

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Runner executes the hostinit process.
type Runner struct{}

const (
	RunnerName  = "hostinit"
	Description = "initialize new Evergreen hosts"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	init := &HostInit{config}

	if err := init.startHosts(); err != nil {
		err = errors.Wrap(err, "Error starting hosts")
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"method":  "startHosts",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})
		return err
	}

	if err := init.setupReadyHosts(); err != nil {
		err = errors.Wrap(err, "Error provisioning hosts")
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"method":  "setupReadyHosts",
			"runtime": time.Since(startTime),
		})
		return err
	}

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(errors.Wrap(err, "problem updating process status"))
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": time.Since(startTime),
		"span":    time.Since(startTime).String(),
		"status":  "success",
	})

	return nil
}
