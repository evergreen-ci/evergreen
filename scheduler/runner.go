package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

// Runner runs the scheduler process.
type Runner struct{}

const (
	// builds per-distro queues of tasks for execution and creates host intents
	RunnerName = "scheduler"
)

func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()
	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.SchedulerDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "scheduler is disabled, exiting",
		})
		return nil
	}
	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	schedulerInstance := &Scheduler{
		config,
		&CmpBasedTaskPrioritizer{},
		&DBTaskDurationEstimator{},
		&DBTaskQueuePersister{},
		&DurationBasedHostAllocator{},
		LegacyFindRunnableTasks,
	}

	switch config.Scheduler.TaskFinder {
	case "parallel":
		schedulerInstance.FindRunnableTasks = ParallelTaskFinder
	case "legacy":
		schedulerInstance.FindRunnableTasks = LegacyFindRunnableTasks
	case "pipeline":
		schedulerInstance.FindRunnableTasks = RunnableTasksPipeline
	case "alternate":
		schedulerInstance.FindRunnableTasks = AlternateTaskFinder
	}

	if err := schedulerInstance.Schedule(ctx); err != nil {
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})

		return errors.Wrap(err, "problem running scheduler")
	}

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(errors.Wrap(err, "problem updating process status"))
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": time.Since(startTime),
		"status":  "success",
		"span":    time.Since(startTime).String(),
	})

	return nil
}
