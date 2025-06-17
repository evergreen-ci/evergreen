package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ClearLocal clears the local test results store.
func ClearLocal(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.DB().Collection(testresult.Collection).Drop(ctx), "clearing the local test results store")
}

// localService implements the local test results service.
type localService struct {
	env evergreen.Environment
}

// NewLocalService returns a local test results service implementation.
func NewLocalService(env evergreen.Environment) *localService {
	return &localService{env: env}
}

// AppendTestResults appends test results to the local test results collection.
func (s *localService) AppendTestResults(ctx context.Context, results []testresult.TestResult) error {
	ids := map[testresult.DbTaskTestResultsID][]testresult.TestResult{}
	for _, result := range results {
		id := testresult.DbTaskTestResultsID{
			TaskID:    result.TaskID,
			Execution: result.Execution,
		}
		ids[id] = append(ids[id], result)
	}

	catcher := grip.NewBasicCatcher()
	for id, results := range ids {
		catcher.Add(s.appendResults(ctx, results, id))
	}
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "appending test results")
	}

	return nil
}

func (s *localService) appendResults(ctx context.Context, results []testresult.TestResult, id testresult.DbTaskTestResultsID) error {
	var failedCount int
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			failedCount++
		}
	}

	update := bson.M{
		"$push": bson.M{testresult.ResultsKey: bson.M{"$each": results}},
		"$inc": bson.M{
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.TotalCountKey):  len(results),
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.FailedCountKey): failedCount,
		},
	}
	_, err := s.env.DB().Collection(testresult.Collection).UpdateOne(ctx, bson.M{IdKey: id}, update, options.Update().SetUpsert(true))
	return errors.Wrap(err, "appending DB test results")
}

func (s *localService) GetTaskTestResults(ctx context.Context, taskOpts []Task, _ []Task) ([]testresult.TaskTestResults, error) {
	allTaskResults, err := s.Get(ctx, taskOpts)
	if err != nil {
		return nil, errors.Wrap(err, "getting local test results")
	}
	return allTaskResults, nil
}

func (s *localService) GetTaskTestResultsStats(ctx context.Context, taskOpts []Task) (testresult.TaskTestResultsStats, error) {
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

func (s *localService) GetFailedTestSamples(ctx context.Context, taskOpts []Task, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	return nil, errors.New("not implemented")
}

// Get fetches the unmerged test results for the given tasks from the local
// store.
func (s *localService) Get(ctx context.Context, taskOpts []Task, fields ...string) ([]testresult.TaskTestResults, error) {
	ids := make([]testresult.DbTaskTestResultsID, len(taskOpts))
	for i, task := range taskOpts {
		ids[i].TaskID = task.Id
		ids[i].Execution = task.Execution
	}

	filter := bson.M{testresult.IdKey: bson.M{"$in": ids}}
	opts := options.Find()
	opts.SetSort(bson.D{{Name: testresult.TaskIDKey, Value: 1}, {Name: testresult.ExecutionKey, Value: 1}})
	if len(fields) > 0 {
		projection := bson.M{}
		for _, field := range fields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	var allDBTaskResults []testresult.DbTaskTestResults
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
