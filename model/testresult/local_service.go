package testresult

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const maxSampleSize = 10

// ClearLocal clears the local test results store.
func ClearLocal(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.DB().Collection(Collection).Drop(ctx), "clearing the local test results store")
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
func (s *localService) AppendTestResults(ctx context.Context, results []TestResult) error {
	ids := map[dbTaskTestResultsID][]TestResult{}
	for _, result := range results {
		id := dbTaskTestResultsID{
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

func (s *localService) GetTaskTestResults(ctx context.Context, taskOpts []TaskOptions, _ []TaskOptions) ([]TaskTestResults, error) {
	allTaskResults, err := s.get(ctx, taskOpts)
	if err != nil {
		return nil, errors.Wrap(err, "getting local test results")
	}
	return allTaskResults, nil
}

func (s *localService) GetTaskTestResultsStats(ctx context.Context, taskOpts []TaskOptions) (TaskTestResultsStats, error) {
	allTaskResults, err := s.get(ctx, taskOpts, statsKey)
	if err != nil {
		return TaskTestResultsStats{}, errors.Wrap(err, "getting local test results")
	}

	var mergedStats TaskTestResultsStats
	for _, taskResults := range allTaskResults {
		mergedStats.TotalCount += taskResults.Stats.TotalCount
		mergedStats.FailedCount += taskResults.Stats.FailedCount
	}

	return mergedStats, nil
}

func (s *localService) GetFailedTestSamples(ctx context.Context, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	return nil, errors.New("not implemented")
}

// get fetches the unmerged test results for the given tasks from the local
// store.
func (s *localService) get(ctx context.Context, taskOpts []TaskOptions, fields ...string) ([]TaskTestResults, error) {
	ids := make([]dbTaskTestResultsID, len(taskOpts))
	for i, task := range taskOpts {
		ids[i].TaskID = task.TaskID
		ids[i].Execution = task.Execution
	}

	filter := bson.M{idKey: bson.M{"$in": ids}}
	opts := options.Find()
	opts.SetSort(bson.D{{Name: taskIDKey, Value: 1}, {Name: executionKey, Value: 1}})
	if len(fields) > 0 {
		projection := bson.M{}
		for _, field := range fields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	var allDBTaskResults []dbTaskTestResults
	cur, err := s.env.DB().Collection(Collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.Wrap(err, "finding DB test results")
	}
	if err = cur.All(ctx, &allDBTaskResults); err != nil {
		return nil, errors.Wrap(err, "reading DB test results")
	}

	allTaskResults := make([]TaskTestResults, len(allDBTaskResults))
	for i, dbTaskResults := range allDBTaskResults {
		allTaskResults[i].Stats = dbTaskResults.Stats
		allTaskResults[i].Results = dbTaskResults.Results
	}

	return allTaskResults, nil
}
