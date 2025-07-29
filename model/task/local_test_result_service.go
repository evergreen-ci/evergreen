package task

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	TestResultTaskIDKey    = bsonutil.MustHaveTag(dbTaskTestResultsID{}, "TaskID")
	TestResultExecutionKey = bsonutil.MustHaveTag(dbTaskTestResultsID{}, "Execution")
	ResultsKey             = bsonutil.MustHaveTag(localDbTaskTestResults{}, "Results")
)

// localTestResultsService implements the local test results service.
type localTestResultsService struct {
	env evergreen.Environment
}

// ClearTestResults clears the local test results store.
func ClearTestResults(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.CedarDB().Collection(testresult.Collection).Drop(ctx), "clearing the local test results store")
}

// NewLocalService returns a local test results service implementation.
func NewLocalService(env evergreen.Environment) *localTestResultsService {
	return &localTestResultsService{env: env}
}

// AppendTestResultMetadata appends test results to the local test results collection.
func (s *localTestResultsService) AppendTestResultMetadata(ctx context.Context, _ []string, failedCount int, totalResults int, tr testresult.DbTaskTestResults) error {
	update := bson.M{
		"$push": bson.M{ResultsKey: bson.M{"$each": tr.Results}},
		"$inc": bson.M{
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.TotalCountKey):  totalResults,
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.FailedCountKey): failedCount,
		},
	}
	id := dbTaskTestResultsID{
		TaskID:    tr.Info.TaskID,
		Execution: tr.Info.Execution,
	}
	_, err := s.env.DB().Collection(testresult.Collection).UpdateOne(ctx, bson.M{IdKey: id}, update, options.Update().SetUpsert(true))
	return errors.Wrap(err, "appending DB test results")
}

func (s *localTestResultsService) GetTaskTestResults(ctx context.Context, taskOpts []Task) ([]testresult.TaskTestResults, error) {
	allTaskResults, err := s.Get(ctx, taskOpts)
	if err != nil {
		return nil, errors.Wrap(err, "getting local test results")
	}
	return allTaskResults, nil
}

func (s *localTestResultsService) GetTaskTestResultsStats(ctx context.Context, taskOpts []Task) (testresult.TaskTestResultsStats, error) {
	allTaskResults, err := s.Get(ctx, taskOpts, testresult.StatsKey)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting local test results")
	}

	var mergedStats testresult.TaskTestResultsStats
	for _, taskResults := range allTaskResults {
		mergedStats.TotalCount += taskResults.Stats.TotalCount
		mergedStats.FailedCount += taskResults.Stats.FailedCount
	}

	return mergedStats, nil
}

// Get fetches the unmerged test results for the given tasks from the local
// store.
func (s *localTestResultsService) Get(ctx context.Context, taskOpts []Task, fields ...string) ([]testresult.TaskTestResults, error) {
	ids := make([]dbTaskTestResultsID, len(taskOpts))
	for i, task := range taskOpts {
		ids[i].TaskID = task.Id
		ids[i].Execution = task.Execution
	}

	filter := bson.M{testresult.IdKey: bson.M{"$in": ids}}
	opts := options.Find()
	opts.SetSort(bson.D{{Name: TestResultTaskIDKey, Value: 1}, {Name: TestResultExecutionKey, Value: 1}})
	if len(fields) > 0 {
		projection := bson.M{}
		for _, field := range fields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	var allDBTaskResults []localDbTaskTestResults
	cur, err := s.env.DB().Collection(testresult.Collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.Wrap(err, "finding DB test results")
	}
	if err = cur.All(ctx, &allDBTaskResults); err != nil {
		return nil, errors.Wrap(err, "reading DB test results")
	}

	allTaskResults := make([]testresult.TaskTestResults, len(allDBTaskResults))
	for i, dbTaskResults := range allDBTaskResults {
		allTaskResults[i].Stats = dbTaskResults.Stats
		allTaskResults[i].Results = dbTaskResults.Results
	}

	return allTaskResults, nil
}

type localDbTaskTestResults struct {
	ID          dbTaskTestResultsID             `bson:"_id"`
	Stats       testresult.TaskTestResultsStats `bson:"stats"`
	Info        testresult.TestResultsInfo      `bson:"info"`
	CreatedAt   time.Time                       `bson:"created_at"`
	CompletedAt time.Time                       `bson:"completed_at"`
	// FailedTestsSample is the first X failing tests of the test Results.
	// This is an optimization for Evergreen's UI features that display a
	// limited number of failing tests for a task.
	FailedTestsSample []string                `bson:"failed_tests_sample"`
	Results           []testresult.TestResult `bson:"results"`
}

type dbTaskTestResultsID struct {
	TaskID    string `bson:"task_id"`
	Execution int    `bson:"execution"`
}
