package task

import (
	"context"
	"runtime"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const failedTestsSampleSize = 10

// ClearTestResults clears the local test results store.
func ClearTestResults(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.CedarDB().Collection(testresult.Collection).Drop(ctx), "clearing the local test results store")
}

// evergreenService implements the local test results service.
type evergreenService struct {
	env evergreen.Environment
}

// NewEvergreenService returns a local test results service implementation.
func NewEvergreenService(env evergreen.Environment) *evergreenService {
	return &evergreenService{env: env}
}

// AppendTestResultMetadata appends test results to the local test results collection.
func (s *evergreenService) AppendTestResultMetadata(ctx context.Context, failedTestSample []string, failedCount int, totalResults int, record testresult.DbTaskTestResults) error {
	updatedFailedSample := record.FailedTestsSample
	for _, sample := range failedTestSample {
		if len(updatedFailedSample) < failedTestsSampleSize {
			updatedFailedSample = append(updatedFailedSample, sample)
		}
	}
	update := bson.M{
		"$inc": bson.M{
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.TotalCountKey):  totalResults,
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.FailedCountKey): failedCount,
		},
		"$set": bson.M{
			testresult.TestResultsFailedTestsSampleKey: updatedFailedSample,
		},
	}
	_, err := s.env.CedarDB().Collection(testresult.Collection).UpdateOne(ctx, bson.M{IdKey: record.ID}, update, options.Update().SetUpsert(true))
	return errors.Wrap(err, "appending DB test results")
}

func (s *evergreenService) GetTaskTestResults(ctx context.Context, taskOpts []Task, _ []Task) ([]testresult.TaskTestResults, error) {
	allTaskResults, err := s.Get(ctx, taskOpts)
	if err != nil {
		return nil, errors.Wrap(err, "getting test results")
	}
	return allTaskResults, nil
}

func (s *evergreenService) GetTaskTestResultsStats(ctx context.Context, taskOpts []Task) (testresult.TaskTestResultsStats, error) {
	allTaskResults, err := s.Get(ctx, taskOpts, testresult.StatsKey)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting test results")
	}

	var mergedStats testresult.TaskTestResultsStats
	for _, taskResults := range allTaskResults {
		mergedStats.TotalCount += taskResults.Stats.TotalCount
		mergedStats.FailedCount += taskResults.Stats.FailedCount
	}

	return mergedStats, nil
}

func (s *evergreenService) GetFailedTestSamples(ctx context.Context, taskOpts []Task, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	return nil, errors.New("not implemented")
}

// Get fetches the unmerged test results for the given tasks from the local
// store.
func (s *evergreenService) Get(ctx context.Context, taskOpts []Task, fields ...string) ([]testresult.TaskTestResults, error) {
	var filter bson.M
	if len(taskOpts) == 1 {
		filter = ByTaskIDAndExecution(taskOpts[0].Id, taskOpts[0].Execution)
	} else {
		findQueries := make([]bson.M, len(taskOpts))
		for i, taskOpt := range taskOpts {
			findQueries[i] = ByTaskIDAndExecution(taskOpt.Id, taskOpt.Execution)
		}
		filter = bson.M{"$or": findQueries}
	}
	opts := options.Find()
	opts.SetSort(bson.D{
		{Key: bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoTaskIDKey), Value: 1},
		{Key: bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoExecutionKey), Value: 1},
	})
	if len(fields) > 0 {
		projection := bson.M{}
		for _, field := range fields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	var allDBTaskResults []testresult.DbTaskTestResults
	cur, err := s.env.CedarDB().Collection(testresult.Collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.Wrap(err, "finding DB test results")
	}
	if err = cur.All(ctx, &allDBTaskResults); err != nil {
		return nil, errors.Wrap(err, "reading DB test results")
	}
	allTaskResults := make([]testresult.TaskTestResults, len(allDBTaskResults))

	// The fields param will have a non-zero length if this request is coming from GetTaskTestResultsStats,
	// in which case we only need to retrieve stats metadata from the cedar DB and do not need
	// to download anything from s3.
	if len(fields) == 0 {
		toDownload := make(chan *testresult.DbTaskTestResults, len(allDBTaskResults))
		for i := range allDBTaskResults {
			toDownload <- &allDBTaskResults[i]
		}
		close(toDownload)

		config, err := evergreen.GetConfig(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "retrieving config")
		}

		var wg sync.WaitGroup
		catcher := grip.NewBasicCatcher()
		for i := 0; i < runtime.NumCPU(); i++ {
			wg.Add(1)
			go workerDownload(ctx, toDownload, config.Buckets.Credentials, catcher, &wg)
		}
		wg.Wait()
		if catcher.HasErrors() {
			return nil, catcher.Resolve()
		}
	}
	for i, dbTaskResults := range allDBTaskResults {
		allTaskResults[i].Stats = dbTaskResults.Stats
		allTaskResults[i].Results = dbTaskResults.Results
	}

	return allTaskResults, nil
}

func workerDownload(ctx context.Context, toDownload <-chan *testresult.DbTaskTestResults, credentials evergreen.S3Credentials, catcher grip.Catcher, wg *sync.WaitGroup) {
	defer func() {
		catcher.Add(recovery.HandlePanicWithError(recover(), nil, "test results download producer"))
		wg.Done()
	}()

	for trs := range toDownload {
		results, err := download(ctx, credentials, trs)
		if err != nil {
			catcher.Add(err)
			return
		}
		trs.Results = results

		if err = ctx.Err(); err != nil {
			catcher.Add(err)
			return
		}
	}
}

// download returns a TestResult slice with the corresponding Results stored in
// the offline blob storage.
func download(ctx context.Context, credentials evergreen.S3Credentials, t *testresult.DbTaskTestResults) ([]testresult.TestResult, error) {
	dbTask, err := FindOneIdAndExecution(ctx, t.Info.TaskID, t.Info.Execution)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task '%s'", t.Info.TaskID)
	}
	if dbTask == nil {
		return nil, errors.Errorf("task '%s' not found", t.Info.TaskID)
	}
	outputInfo, ok := dbTask.GetTaskOutputSafe()
	if !ok {
		return nil, nil
	}
	return outputInfo.TestResults.downloadParquet(ctx, credentials, t)
}

// ByTaskIDAndExecution constructs a query to find a test result for a specific task id and execution pair.
func ByTaskIDAndExecution(id string, execution int) bson.M {
	return bson.M{
		bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoTaskIDKey):    id,
		bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoExecutionKey): execution,
	}
}
