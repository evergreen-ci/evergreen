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

func (s *localService) appendResults(ctx context.Context, results []TestResult, id dbTaskTestResultsID) error {
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
	_, err := s.env.DB().Collection(Collection).UpdateOne(ctx, bson.M{idKey: id}, update, options.Update().SetUpsert(true))
	return errors.Wrap(err, "appending DB test results")
}
