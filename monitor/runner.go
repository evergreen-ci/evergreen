package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Runner struct{}

const (
	RunnerName  = "monitor"
	Description = "track and clean up expired hosts and tasks"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Infoln("Starting monitor at time", startTime)

	if err := RunAllMonitoring(config); err != nil {
		err = errors.Wrap(err, "error running monitor")
		grip.Error(err)
		return err
	}

	runtime := time.Since(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		grip.Errorf("error updating process status: %+v", err)
	}
	grip.Infof("Monitor took %s to run", runtime)
	return nil
}
