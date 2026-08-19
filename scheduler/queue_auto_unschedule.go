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

// autoUnscheduleLargeQueue unschedules every patch tasks in a distro's queue once the plan
// reaches the threshold.
func autoUnscheduleLargeQueue(ctx context.Context, distroID string, plan []task.Task, threshold int) error {
	if threshold <= 0 || len(plan) < threshold {
		return nil
	}

	tasksToUnschedule := make([]task.Task, 0, len(plan))
	for _, t := range plan {
		if t.Requester == evergreen.PatchVersionRequester {
			tasksToUnschedule = append(tasksToUnschedule, t)
		}
	}
	if len(tasksToUnschedule) == 0 {
		return nil
	}

	if err := task.DeactivateTasks(ctx, tasksToUnschedule, true, evergreen.OversizedQueueUnscheduler, ""); err != nil {
		return errors.Wrap(err, "unscheduling patch tasks")
	}

	taskIDs := make([]string, 0, len(tasksToUnschedule))
	for _, victim := range tasksToUnschedule {
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
	grip.Error(ctx, message.Fields{
		"message":      "task queue is too long and CLI patch tasks have been auto-unscheduled",
		"distro":       distroID,
		"threshold":    threshold,
		"queue_length": len(plan),
		"unscheduling": len(tasksToUnschedule),
	})

	return nil
}
