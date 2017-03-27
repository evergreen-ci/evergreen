package taskrunner

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Runner struct{}

const (
	RunnerName  = "taskrunner"
	Description = "run queued tasks on available hosts"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Infoln("Starting taskrunner at time", startTime)

	if err := NewTaskRunner(config).Run(); err != nil {
		err = errors.Wrap(err, "error running taskrunner")
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		grip.Errorln("error updating process status:", err)
	}

	grip.Infof("Taskrunner took %s to run", runtime)
	return nil
}
