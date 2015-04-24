package hostinit

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

type Runner struct{}

const (
	RunnerName = "hostinit"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *mci.MCISettings) error {
	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting hostinit at time %v", startTime)

	init := &HostInit{config}

	if err := init.setupReadyHosts(); err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "Error running hostinit: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Hostinit took %v to run", runtime)
	return nil
}
