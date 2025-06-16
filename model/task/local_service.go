package task

import (
	"context"
	adb "github.com/mongodb/anser/db"
	"runtime"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const failedTestsSampleSize = 10

// ClearLocal clears the local test results store.
func ClearLocal(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.CedarDB().Collection(testresult.Collection).Drop(ctx), "clearing the local test results store")
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
	infos := map[testresult.TestResultsInfo][]testresult.TestResult{}
	for _, result := range results {
		info := testresult.TestResultsInfo{
			TaskID:    result.TaskID,
			Execution: result.Execution,
		}
		infos[info] = append(infos[info], result)
	}

	catcher := grip.NewBasicCatcher()
	for info := range infos {
		catcher.Add(s.appendResults(ctx, results, info))
	}
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "appending test results")
	}

	return nil
}

func (s *localService) appendResults(ctx context.Context, results []testresult.TestResult, info testresult.TestResultsInfo) error {
	record := testresult.DbTaskTestResults{
		ID:   info.ID(),
		Info: info,
	}
	err := s.env.CedarDB().Collection(testresult.Collection).FindOne(ctx, createFindQuery(info.TaskID, info.Execution)).Decode(&record)
	if err != nil && !adb.ResultsNotFound(err) {
		return errors.Wrapf(err, "finding test result '%s' execution '%d'", info.TaskID, info.Execution)
	}

	var failedCount int
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			if len(record.FailedTestsSample) < failedTestsSampleSize {
				record.FailedTestsSample = append(record.FailedTestsSample, result.GetDisplayTestName())
			}
			failedCount++
		}
	}

	update := bson.M{
		"$inc": bson.M{
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.TotalCountKey):  len(results),
			bsonutil.GetDottedKeyName(testresult.StatsKey, testresult.FailedCountKey): failedCount,
		},
		"$set": bson.M{
			testresult.TestResultsFailedTestsSampleKey: record.FailedTestsSample,
		},
	}
	_, err = s.env.CedarDB().Collection(testresult.Collection).UpdateOne(ctx, bson.M{IdKey: record.ID}, update, options.Update().SetUpsert(true))
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
	var filter bson.M
	if len(taskOpts) == 1 {
		filter = createFindQuery(taskOpts[0].Id, taskOpts[0].Execution)
	} else {
		findQueries := make([]bson.M, len(taskOpts))
		for i, taskOpt := range taskOpts {
			findQueries[i] = createFindQuery(taskOpt.Id, taskOpt.Execution)
		}
		filter = bson.M{"$or": findQueries}
	}
	opts := options.Find()
	opts.SetSort(mgobson.D{
		{Name: bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoTaskIDKey), Value: 1},
		{Name: bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoExecutionKey), Value: 1}},
	)
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
		go func() {
			defer func() {
				catcher.Add(recovery.HandlePanicWithError(recover(), nil, "test results download producer"))
				wg.Done()
			}()

			for trs := range toDownload {
				results, err := download(ctx, config.Buckets.Credentials, trs)
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
		}()
	}
	wg.Wait()
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	for i, dbTaskResults := range allDBTaskResults {
		allTaskResults[i].Stats = dbTaskResults.Stats
		allTaskResults[i].Results = dbTaskResults.Results
	}

	return allTaskResults, nil
}

// download returns a TestResult slice with the corresponding Results stored in
// the offline blob storage. The TestResults record should be populated and the
// environment should not be nil.
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

func createFindQuery(id string, execution int) bson.M {
	return bson.M{
		bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoTaskIDKey):    id,
		bsonutil.GetDottedKeyName(testresult.TestResultsInfoKey, testresult.TestResultsInfoExecutionKey): execution,
	}
}
