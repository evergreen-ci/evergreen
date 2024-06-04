package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	lookBackTime = 7 * 24 * time.Hour // one week
)

type generateTasksEstimationsResults struct {
	DisplayName        string `bson:"_id"`
	EstimatedCreated   int    `bson:"est_created"`
	EstimatedActivated int    `bson:"est_activated"`
}

func (t *Task) setGenerateTasksEstimations() error {
	// Do not run if estimations have already been cached.
	if t.EstimatedNumGeneratedTasks != nil && t.EstimatedNumActivatedGeneratedTasks != nil {
		return nil
	}

	match := bson.M{
		BuildVariantKey:   t.BuildVariant,
		ProjectKey:        t.Project,
		DisplayNameKey:    t.DisplayName,
		GeneratedTasksKey: true,
		StatusKey: bson.M{
			"$in": evergreen.TaskCompletedStatuses,
		},
		StartTimeKey: bson.M{
			"$gt": time.Now().Add(-1 * lookBackTime),
		},
		FinishTimeKey: bson.M{
			"$lte": time.Now(),
		},
	}

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$project": bson.M{
				DisplayNameKey:                1,
				NumGeneratedTasksKey:          1,
				NumActivatedGeneratedTasksKey: 1,
				IdKey:                         0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%s", DisplayNameKey),
				"est_created": bson.M{
					"$avg": fmt.Sprintf("$%s", NumGeneratedTasksKey),
				},
				"est_activated": bson.M{
					"$avg": fmt.Sprintf("$%s", NumActivatedGeneratedTasksKey),
				},
			},
		},
	}

	// anonymous struct for unmarshalling result bson
	results := []generateTasksEstimationsResults{}

	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	cursor, err := coll.Aggregate(ctx, pipeline, &options.AggregateOptions{})
	if err != nil {
		return errors.Wrap(err, "aggregating generate tasks estimations")
	}
	err = cursor.All(ctx, &results)
	if err != nil {
		return errors.Wrap(err, "iterating and decoding generate tasks estimations")
	}

	if len(results) != 1 {
		if len(results) == 0 {
			t.EstimatedNumGeneratedTasks = utility.ToIntPtr(0)
			t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(0)
		} else {
			return errors.New("unexpected number of results from generate tasks estimations aggregation")
		}
	} else {
		t.EstimatedNumGeneratedTasks = utility.ToIntPtr(results[0].EstimatedCreated)
		t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(results[0].EstimatedActivated)
	}

	if err = t.cacheGenerateTasksEstimations(); err != nil {
		return errors.Wrap(err, "caching generate tasks estimations")
	}

	return nil
}

// func (t *Task) setGenerateTasksEstimations() error {
// 	if !t.GenerateTask || (t.EstimatedNumGeneratedTasks != nil && t.EstimatedNumActivatedGeneratedTasks != nil) {
// 		return nil
// 	}

// 	query := db.Query(
// 		ByPreviousCommit(
// 			t.BuildVariant,
// 			t.DisplayName,
// 			t.Project,
// 			evergreen.RepotrackerVersionRequester,
// 			t.RevisionOrderNumber,
// 		),
// 	).Sort([]string{"-" + RevisionOrderNumberKey}).Limit(numRevisionsToSearch)

// 	tasks := []Task{}
// 	err := db.FindAllQ(Collection, query, &tasks)
// 	if err != nil {
// 		return errors.Wrapf(err, "finding tasks named '%s' in '%s'", t.DisplayName, t.Project)
// 	}
// 	if len(tasks) == 0 {
// 		return nil
// 	}

// 	generatedTotal := 0
// 	activatedTotal := 0
// 	for _, task := range tasks {
// 		generatedTotal += task.NumGeneratedTasks
// 		activatedTotal += task.NumActivatedGeneratedTasks
// 	}

// 	t.EstimatedNumGeneratedTasks = utility.ToIntPtr(generatedTotal / len(tasks))
// 	t.EstimatedNumActivatedGeneratedTasks = utility.ToIntPtr(activatedTotal / len(tasks))

// 	if err = t.cacheGenerateTasksEstimations(); err != nil {
// 		return errors.Wrap(err, "caching generate tasks estimations")
// 	}

// 	return nil
// }

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
