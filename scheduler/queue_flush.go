package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// flushOversizedQueue unschedules every CLI patch task in a distro's plan once the plan
// reaches threshold, to stop them accumulating in a queue that cannot dispatch them.
//
// Only PatchVersionRequester tasks are eligible. Everything else, including GitHub PR and
// merge queue tasks, is left alone, so a distro whose other demand alone reaches the
// threshold stays over it; that case is logged for on-call instead.
func flushOversizedQueue(ctx context.Context, distroID string, plan []task.Task, threshold int) error {
	if threshold <= 0 || len(plan) < threshold {
		return nil
	}

	victims := make([]task.Task, 0, len(plan))
	for _, t := range plan {
		if t.Requester == evergreen.PatchVersionRequester {
			victims = append(victims, t)
		}
	}

	grip.Warning(ctx, message.Fields{
		"message":      "task queue is oversized",
		"distro":       distroID,
		"runner":       RunnerName,
		"planned":      len(plan),
		"threshold":    threshold,
		"unscheduling": len(victims),
	})
	if len(victims) == 0 {
		return nil
	}

	if err := task.DeactivateTasks(ctx, victims, true, evergreen.OversizedQueueUnscheduler); err != nil {
		return errors.Wrap(err, "unscheduling patch tasks")
	}

	taskIDs := make([]string, 0, len(victims))
	for _, victim := range victims {
		taskIDs = append(taskIDs, victim.Id)
		if err := model.UpdateBuildAndVersionStatusForTask(ctx, &victim); err != nil {
			return errors.Wrapf(err, "updating build and version status for task '%s'", victim.Id)
		}
		if victim.IsPartOfDisplay(ctx) {
			if err := model.UpdateDisplayTaskForTask(ctx, &victim); err != nil {
				return errors.Wrap(err, "updating parent display task")
			}
		}
	}
	grip.Info(ctx, message.Fields{
		"message":  "flushed patch tasks from an oversized queue",
		"distro":   distroID,
		"runner":   RunnerName,
		"task_ids": taskIDs,
	})

	return nil
}
