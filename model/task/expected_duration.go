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

// this is about a month.
const oneMonthIsh = 30 * 24 * time.Hour
const durationIndex = "branch_1_build_variant_1_display_name_1_status_1_finish_time_1_start_time_1"

type expectedDurationResults struct {
	DisplayName      string  `bson:"_id"`
	ExpectedDuration float64 `bson:"exp_dur"`
	StdDev           float64 `bson:"std_dev"`
}

func getExpectedDurationsForWindow(name, project, buildvariant string, start, end time.Time) ([]expectedDurationResults, error) {
	match := bson.M{
		BuildVariantKey: buildvariant,
		ProjectKey:      project,
		StatusKey: bson.M{
			"$in": []string{evergreen.TaskSucceeded, evergreen.TaskFailed},
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
	cursor, err := coll.Aggregate(ctx, pipeline, &options.AggregateOptions{Hint: durationIndex})
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task average duration")
	}
	err = cursor.All(ctx, &results)
	if err != nil {
		return nil, errors.Wrap(err, "iterating and decoding task average duration")
	}

	return results, nil
}

// ExpectedTaskDuration takes a given project and buildvariant and computes
// the average duration - grouped by task display name - for tasks that have
// completed within a given threshold as determined by the window
func ExpectedTaskDuration(project, buildvariant string, window time.Duration) (map[string]time.Duration, error) {
	results, err := getExpectedDurationsForWindow("", project, buildvariant, time.Now().Add(-window), time.Now())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	expDurations := make(map[string]time.Duration)
	for _, result := range results {
		expDuration := time.Duration(result.ExpectedDuration) * time.Nanosecond
		expDurations[result.DisplayName] = expDuration
	}

	return expDurations, nil
}
