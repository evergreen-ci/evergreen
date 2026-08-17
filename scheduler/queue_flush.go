package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// oversizedQueueGracePeriod is how long a task must have been activated before an
	// oversized queue will unschedule it, so that a burst of freshly scheduled work is
	// never flushed on the spike that created it.
	oversizedQueueGracePeriod = time.Hour

	// maxTasksFlushedPerPass bounds how many tasks one pass may unschedule, since each
	// one costs a build and version status update. A queue needing more converges over
	// subsequent passes.
	// ponytail: a fixed ceiling, make it a setting if a real flush can't keep up.
	maxTasksFlushedPerPass = 1000
)

// flushOversizedQueue unschedules the patch tasks overflowing a distro's queue once the
// plan grows past threshold, to stop them accumulating in a queue that cannot dispatch
// them. Only tasks past the threshold are eligible, so the tasks that are actually
// dispatching are never touched. plan must be in scheduler-sorted order.
//
// Mainline and merge queue tasks are never unscheduled, so a distro whose mainline demand
// alone exceeds the threshold stays over it; that case is logged for on-call instead.
func flushOversizedQueue(ctx context.Context, distroID string, plan []task.Task, threshold int) error {
	if threshold <= 0 || len(plan) <= threshold {
		return nil
	}

	staleBefore := time.Now().Add(-oversizedQueueGracePeriod)
	overflow := plan[threshold:]
	victims := make([]task.Task, 0, len(overflow))
	// Walk back to front so the lowest-ranked tasks are flushed first.
	for i := len(overflow) - 1; i >= 0 && len(victims) < maxTasksFlushedPerPass; i-- {
		if t := overflow[i]; isFlushable(t) && t.ActivatedTime.Before(staleBefore) {
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
		return errors.Wrap(err, "unscheduling overflowing tasks")
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
		"message":  "unscheduled tasks overflowing an oversized queue",
		"distro":   distroID,
		"runner":   RunnerName,
		"task_ids": taskIDs,
	})

	return nil
}

// isFlushable reports whether an overflowing task may be unscheduled to shrink a queue.
// Merge queue tasks are excluded even though they are patches, because unscheduling one
// stalls the merge it belongs to.
func isFlushable(t task.Task) bool {
	return evergreen.IsPatchRequester(t.Requester) && t.Requester != evergreen.GithubMergeRequester
}
