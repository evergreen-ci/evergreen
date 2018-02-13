package task

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// this is about a month.
const oneMonthIsh = 30 * 24 * time.Hour

type expectedDurationResults struct {
	DisplayName      string `bson:"_id"`
	ExpectedDuration int64  `bson:"exp_dur"`
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
	}

	if name != "" {
		match[DisplayNameKey] = name
	}

	// If we pass zerotime as a starting point then the end time
	// means "all tasks that finshed after this point," otherwise
	// it's a window in the conventional sense. The scheduler and
	// the public function (ExpectedTaskDuration) use the
	// unconventional sense of window.
	if start == util.ZeroTime {
		match[FinishTimeKey] = bson.M{
			"$gte": end,
		}
	} else {
		match[FinishTimeKey] = bson.M{
			"$lte": end,
		}
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
				"_id": fmt.Sprintf("$%v", DisplayNameKey),
				"exp_dur": bson.M{
					"$avg": fmt.Sprintf("$%v", TimeTakenKey),
				},
			},
		},
	}

	// anonymous struct for unmarshalling result bson
	results := []expectedDurationResults{}

	err := db.Aggregate(Collection, pipeline, &results)
	if err != nil {
		return nil, errors.Wrap(err, "error aggregating task average duration")
	}

	return results, nil
}

// ExpectedTaskDuration takes a given project and buildvariant and computes
// the average duration - grouped by task display name - for tasks that have
// completed within a given threshold as determined by the window
func ExpectedTaskDuration(project, buildvariant string, window time.Duration) (map[string]time.Duration, error) {
	results, err := getExpectedDurationsForWindow("", project, buildvariant, util.ZeroTime, time.Now().Add(-window))
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
