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

// estimateCacheMaxSize bounds each shared historical-estimate cache to roughly
// 12MB, at ~250 bytes per (project, build variant, task name) entry. Peak live
// entries over a 24-hour slow log were ~700 for durations and ~3.8k for
// generate estimations, and that undersamples projects fast enough to stay out
// of the log. Overflowing costs a recompute, not correctness.
const estimateCacheMaxSize = 50000

// expectedDurationCache shares the historical duration aggregate across sibling
// tasks, which Task.DurationPrediction cannot do because it is keyed by task
// document.
//
// Adopting a shared entry restarts the task's own predictionTTL clock, so the
// estimate a task holds can be up to 2*predictionTTL old rather than
// predictionTTL. That is acceptable because the underlying value is a
// seven-day trailing average, which barely moves over a day.
var expectedDurationCache = expirable.NewLRU[string, util.DurationStats](estimateCacheMaxSize, nil, predictionTTL)

func estimateCacheKey(project, buildVariant, displayName string) string {
	return strings.Join([]string{project, buildVariant, displayName}, "\x00")
}

// ClearEstimateCaches drops the process-local historical-estimate caches. It's
// exported for tests in other packages that assert on estimates computed from
// task history, since a stale shared entry would make them order-dependent.
func ClearEstimateCaches() {
	expectedDurationCache.Purge()
	generateTasksEstimationCache.Purge()
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
