package testresult

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const Collection = "testresults"

type dbTaskTestResults struct {
	ID      dbTaskTestResultsID  `bson:"_id"`
	Stats   TaskTestResultsStats `bson:"stats"`
	Results []TestResult         `bson:"results"`
}

type dbTaskTestResultsID struct {
	TaskID    string `bson:"task_id"`
	Execution int    `bson:"execution"`
}

var (
	idKey      = bsonutil.MustHaveTag(dbTaskTestResults{}, "ID")
	statsKey   = bsonutil.MustHaveTag(dbTaskTestResults{}, "Stats")
	resultsKey = bsonutil.MustHaveTag(dbTaskTestResults{}, "Results")

	taskIDKey    = bsonutil.MustHaveTag(dbTaskTestResultsID{}, "TaskID")
	executionKey = bsonutil.MustHaveTag(dbTaskTestResultsID{}, "Execution")

	totalCountKey  = bsonutil.MustHaveTag(TaskTestResultsStats{}, "TotalCount")
	failedCountKey = bsonutil.MustHaveTag(TaskTestResultsStats{}, "FailedCount")
)

func (id dbTaskTestResultsID) appendResults(ctx context.Context, env evergreen.Environment, results []TestResult) error {
	var failedCount int
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			failedCount++
		}
	}

	update := bson.M{
		"$push": bson.M{resultsKey: bson.M{"$each": results}},
		"$inc": bson.M{
			bsonutil.GetDottedKeyName(statsKey, totalCountKey):  len(results),
			bsonutil.GetDottedKeyName(statsKey, failedCountKey): failedCount,
		},
	}
	_, err := env.DB().Collection(Collection).UpdateOne(ctx, bson.M{idKey: id}, update, options.Update().SetUpsert(true))
	return errors.Wrap(err, "appending DB test results")
}

func appendDBResults(ctx context.Context, env evergreen.Environment, results []TestResult) error {
	ids := map[dbTaskTestResultsID][]TestResult{}
	for _, result := range results {
		id := dbTaskTestResultsID{
			TaskID:    result.TaskID,
			Execution: result.Execution,
		}
		ids[id] = append(ids[id], result)
	}

	for id, results := range ids {
		if err := id.appendResults(ctx, env, results); err != nil {
			return err
		}
	}

	return nil
}
