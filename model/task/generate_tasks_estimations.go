package task

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

const (
	lookBackTime = 7 * 24 * time.Hour // one week
)

// SetGenerateTasksEstimations calculates and caches the estimated number of tasks that this task will generate.
// To be called only in task creation.
func (t *Task) SetGenerateTasksEstimations(ctx context.Context) error {
	// Do not run if the task is not a generator.
	if !t.GenerateTask {
		return nil
	}

	results, err := getGenerateTasksEstimation(ctx, t.Project, t.BuildVariant, t.DisplayName, lookBackTime)
	if err != nil {
		return errors.Wrap(err, "getting generate tasks estimation")
	}

	if len(results) == 0 {
		t.EstimatedNumGeneratedTasks = utility.ToIntPtr(0)
		t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(0)

		return nil
	} else if len(results) > 1 {
		return errors.Errorf("expected 1 result from generate tasks estimations aggregation but got %d", len(results))
	}

	t.EstimatedNumGeneratedTasks = utility.ToIntPtr(int(math.Round(results[0].EstimatedCreated)))
	t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(int(math.Round(results[0].EstimatedActivated)))

	return nil
}
