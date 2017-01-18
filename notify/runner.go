package notify

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/tychoish/grip"
)

type Runner struct{}

const (
	RunnerName  = "notify"
	Description = "send notifications for failed tasks and system issues"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Infoln("Starting notifications at time", startTime)

	if err := Run(config); err != nil {
		err = fmt.Errorf("error running notify: %+v", err)
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		grip.Errorln("error updating process status:", err)
	}
	grip.Infoln("Notify took %v to run", runtime)
	return nil
}
