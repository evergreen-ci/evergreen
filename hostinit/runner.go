package hostinit

import (
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
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
	evergreen.Logger.Logf(slogger.INFO, "Starting hostinit at time %v", startTime)

	init := &HostInit{config}

	if err := init.setupReadyHosts(); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error running hostinit: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Hostinit took %v to run", runtime)
	return nil
}
