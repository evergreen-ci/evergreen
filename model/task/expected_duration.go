package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var TaskHistoricalDataIndex = bson.D{
	{Key: ProjectKey, Value: 1},
	{Key: BuildVariantKey, Value: 1},
	{Key: DisplayNameKey, Value: 1},
	{Key: StatusKey, Value: 1},
	{Key: FinishTimeKey, Value: 1},
	{Key: StartTimeKey, Value: 1},
}

var TaskVersionCostIndex = bson.D{
	{Key: VersionKey, Value: 1},
	{Key: DisplayOnlyKey, Value: 1},
}

type expectedDurationResults struct {
	DisplayName      string  `bson:"_id"`
	ExpectedDuration float64 `bson:"exp_dur"`
	StdDev           float64 `bson:"std_dev"`
}

func getExpectedDurationsForWindow(name, project, buildVariant string, start, end time.Time) ([]expectedDurationResults, error) {
	match := bson.M{
		BuildVariantKey: buildVariant,
		ProjectKey:      project,
		StatusKey: bson.M{
			"$in": evergreen.TaskCompletedStatuses,
		},
		bsonutil.GetDottedKeyName(DetailsKey, TaskEndDetailTimedOut): bson.M{
			"$ne": true,
		},
		StartTimeKey: bson.M{
			"$gt": start,
		},
		FinishTimeKey: bson.M{
			"$lte": end,
		},
	}

	if name != "" {
		match[DisplayNameKey] = name
	}

	pipeline := []bson.M{
		{
			"$match": match,
		},
		{
			"$project": bson.M{
				DisplayNameKey: 1,
				TimeTakenKey:   1,
				IdKey:          0,
			},
		},
		{
			"$group": bson.M{
				"_id": fmt.Sprintf("$%s", DisplayNameKey),
				"exp_dur": bson.M{
					"$avg": fmt.Sprintf("$%s", TimeTakenKey),
				},
				"std_dev": bson.M{
					"$stdDevPop": fmt.Sprintf("$%s", TimeTakenKey),
				},
			},
		},
	}

	// anonymous struct for unmarshalling result bson
	results := []expectedDurationResults{}

	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	cursor, err := coll.Aggregate(ctx, pipeline, options.Aggregate().SetHint(TaskHistoricalDataIndex))
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task average duration")
	}
	err = cursor.All(ctx, &results)
	if err != nil {
		return nil, errors.Wrap(err, "iterating and decoding task average duration")
	}

	return results, nil
}
