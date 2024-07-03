package task

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	lookBackTime = 7 * 24 * time.Hour // one week
)

func (t *Task) SetGenerateTasksEstimations(ctx context.Context) error {
	// Do not run if the task is not a generator or estimations have already been cached.
	if !t.GenerateTask || (t.EstimatedNumGeneratedTasks != nil && t.EstimatedNumActivatedGeneratedTasks != nil) {
		return nil
	}

	results, err := getGenerateTasksEstimation(ctx, t.Project, t.BuildVariant, t.DisplayName, lookBackTime)
	if err != nil {
		return errors.Wrap(err, "getting generate tasks estimation")
	}

	if len(results) == 0 {
		t.EstimatedNumGeneratedTasks = utility.ToIntPtr(0)
		t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(0)
	} else if len(results) > 1 {
		return errors.Errorf("expected 1 result from generate tasks estimations aggregation but got %d", len(results))
	} else {
		t.EstimatedNumGeneratedTasks = utility.ToIntPtr(int(math.Round(results[0].EstimatedCreated)))
		t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(int(math.Round(results[0].EstimatedActivated)))
	}

	if err = t.cacheGenerateTasksEstimations(); err != nil {
		return errors.Wrap(err, "caching generate tasks estimations")
	}

	return nil
}

func (t *Task) cacheGenerateTasksEstimations() error {
	return UpdateOne(
		bson.M{
			IdKey: t.Id,
		},
		bson.M{
			"$set": bson.M{
				EstimatedNumGeneratedTasksKey:          t.EstimatedNumGeneratedTasks,
				EstimatedNumActivatedGeneratedTasksKey: t.EstimatedNumActivatedGeneratedTasks,
			},
		},
	)
}
