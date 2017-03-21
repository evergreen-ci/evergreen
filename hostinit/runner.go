package hostinit

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
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
	grip.Infof("starting hostinit at time: %s", startTime)

	init := &HostInit{config}

	if err := init.setupReadyHosts(); err != nil {
		err = fmt.Errorf("Error running hostinit: %+v", err)
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		grip.Errorf("Error updating process status: %+v", err)
	}
	grip.Infof("Hostinit took %s to run", runtime)
	return nil
}
