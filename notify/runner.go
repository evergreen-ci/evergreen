package notify

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
	grip.Infof("Starting notifications at %s time", startTime)

	if err := Run(config); err != nil {
		err = errors.Wrap(err, "error running notify")
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		grip.Errorln("error updating process status:", err.Error())
	}
	grip.Infof("Notify took %s to run", runtime)
	return nil
}
