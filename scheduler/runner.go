package scheduler

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Runner runs the scheduler process.
type Runner struct{}

const (
	RunnerName  = "scheduler"
	Description = "queue tasks for execution and allocate hosts"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Infoln("Starting scheduler at time:", startTime)

	schedulerInstance := &Scheduler{
		config,
		&DBTaskFinder{},
		&CmpBasedTaskPrioritizer{},
		&DBTaskDurationEstimator{},
		&DBTaskQueuePersister{},
		&DurationBasedHostAllocator{},
	}

	if err := schedulerInstance.Schedule(); err != nil {
		err = errors.Wrap(err, "Error running scheduler")
		grip.Error(err)
		return err
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		err = errors.Wrap(err, "Error updating process status")
		grip.Error(err)
	}
	grip.Infof("Scheduler took %s to run", runtime)
	return nil
}
