package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// estimateCacheMaxSize bounds the shared historical-estimate caches. Keys are
// active (project, build variant, task name) tuples, so this is far above the
// working set of a busy app server.
const estimateCacheMaxSize = 10000

// expectedDurationCache shares the historical duration aggregate across sibling
// tasks. Task.DurationPrediction already caches this value, but it's keyed by
// task document, so every newly created task re-ran the identical seven-day
// aggregate; a high-throughput generator recomputed it dozens of times a
// minute. The TTL matches the per-task one, so this introduces no new
// staleness, only a coarser cache key.
var expectedDurationCache = expirable.NewLRU[string, util.DurationStats](estimateCacheMaxSize, nil, predictionTTL)

func estimateCacheKey(project, buildVariant, displayName string) string {
	return strings.Join([]string{project, buildVariant, displayName}, "\x00")
}

var TaskHistoricalDataIndex = bson.D{
	{Key: ProjectKey, Value: 1},
	{Key: BuildVariantKey, Value: 1},
	{Key: DisplayNameKey, Value: 1},
	{Key: StatusKey, Value: 1},
	{Key: FinishTimeKey, Value: 1},
	{Key: StartTimeKey, Value: 1},
}

// TaskVersionCostIndex is used to efficiently aggregate task costs by version.
var TaskVersionCostIndex = bson.D{
	{Key: VersionKey, Value: 1},
	{Key: DisplayOnlyKey, Value: 1},
}

type expectedDurationResults struct {
	DisplayName      string  `bson:"_id"`
	ExpectedDuration float64 `bson:"exp_dur"`
	StdDev           float64 `bson:"std_dev"`
}

func getExpectedDurationsForWindow(ctx context.Context, name, project, buildVariant string, start, end time.Time) ([]expectedDurationResults, error) {
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
