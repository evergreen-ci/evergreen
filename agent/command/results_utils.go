package command

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/pkg/errors"
)

// sendTestResults sends the test results to the backend results service.
func sendTestResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, results []testresult.TestResult) error {
	if len(results) == 0 {
		return errors.New("cannot send nil results")
	}

	logger.Task().Info("Attaching test results...")
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := attachTestResults(ctx, conf, td, comm, results); err != nil {
		return errors.Wrap(err, "sending test results")
	}

	logger.Task().Info("Successfully attached results.")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to backend
// logging and results services. Test logs are uploaded in parallel using a
// worker pool for improved performance.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, logs []testlog.TestLog, results []testresult.TestResult) error {
	if len(logs) > 0 {
		logger.Task().Info("Posting test logs...")

		work := make(chan *testlog.TestLog, len(logs))
		for i := range logs {
			work <- &logs[i]
		}
		close(work)

		var succeeded int64
		var wg sync.WaitGroup
		opts := redactor.RedactionOptions{
			Expansions:         conf.NewExpansions,
			Redacted:           conf.Redacted,
			InternalRedactions: conf.InternalRedactions,
		}

		numWorkers := runtime.GOMAXPROCS(0)
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for log := range work {
					if err := ctx.Err(); err != nil {
						logger.Task().Warning(errors.Wrap(err, "context canceled while sending test logs"))
						return
					}
					if err := taskoutput.AppendTestLog(ctx, &conf.Task, opts, log); err != nil {
						logger.Task().Error(errors.Wrap(err, "sending test log"))
						continue
					}
					atomic.AddInt64(&succeeded, 1)

					// Yield to allow other goroutines to run and prevent starvation
					// in intense log uploading workflows.
					runtime.Gosched()
				}
			}()
		}
		wg.Wait()

		logger.Task().Infof("Finished posting test logs (%d of %d succeeded).", succeeded, len(logs))
	}

	return sendTestResults(ctx, comm, logger, conf, results)
}

func attachTestResults(ctx context.Context, conf *internal.TaskConfig, td client.TaskData, comm client.Communicator, results []testresult.TestResult) error {
	output, ok := conf.Task.GetTaskOutputSafe()
	if !ok || output == nil {
		return errors.New("cannot attach test results without a task output")
	}
	switch output.TestResults.Version {
	case task.TestResultServiceCedar, task.TestResultServiceEvergreen:
		failed, err := uploadTestResults(ctx, comm, conf, results, td, output)
		if err != nil {
			return errors.Wrap(err, "attaching test results")
		}
		conf.HasTestResults = true
		if err := comm.SetResultsInfo(ctx, td, failed); err != nil {
			return errors.Wrap(err, "setting results info in the task")
		}
		if failed {
			conf.HasFailingTestResult = true
		}
		return nil
	default:
		return errors.New("invalid test results version")
	}
}

func uploadTestResults(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, results []testresult.TestResult, td client.TaskData, output *task.TaskOutput) (bool, error) {
	createdAt := conf.Task.CreateTime
	info := makeTestResultsInfo(conf.Task, conf.DisplayTaskInfo)
	newResults := makeTestResults(&conf.Task, results)
	key := testresult.PartitionKey(createdAt, info.Project, info.ID())

	tr := &testresult.DbTaskTestResults{
		ID:        info.ID(),
		CreatedAt: createdAt,
		Info:      info,
	}
	allResults, err := output.TestResults.DownloadParquet(ctx, conf.TaskOutput, tr)
	if err != nil && !pail.IsKeyNotFoundError(err) {
		return false, errors.Wrap(err, "getting uploaded test results")
	}
	allResults = append(allResults, newResults...)

	if err = uploadParquet(ctx, conf.TaskOutput, *output, convertToParquet(allResults, info, createdAt), key); err != nil {
		return false, errors.Wrap(err, "uploading parquet test results")
	}

	var failedCount int
	var failedTests []string
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			failedTests = append(failedTests, result.GetDisplayTestName())
			failedCount++
		}
	}
	tr.Stats = testresult.TaskTestResultsStats{
		FailedCount: failedCount,
		TotalCount:  len(newResults),
	}
	tr.FailedTestsSample = failedTests

	if err = comm.SendTestResults(ctx, td, tr); err != nil {
		return false, errors.Wrap(err, "sending test results")
	}
	return tr.Stats.FailedCount > 0, nil
}

func uploadParquet(ctx context.Context, credentials evergreen.S3Credentials, output task.TaskOutput, results *testresult.ParquetTestResults, key string) error {
	bucket, err := output.TestResults.GetBucket(ctx, credentials)
	if err != nil {
		return err
	}
	w, err := bucket.Writer(ctx, key)
	if err != nil {
		return errors.Wrap(err, "creating Presto bucket writer")
	}
	defer w.Close()

	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(task.ParquetTestResultsSchemaDef)))
	defer pw.Close()

	return errors.Wrap(pw.Write(results), "writing Parquet test results")
}

func makeTestResultsInfo(t task.Task, displayTaskInfo *apimodels.DisplayTaskInfo) testresult.TestResultsInfo {
	return testresult.TestResultsInfo{
		Project:         t.Project,
		Version:         t.Version,
		Variant:         t.BuildVariant,
		TaskID:          t.Id,
		TaskName:        t.DisplayName,
		DisplayTaskID:   displayTaskInfo.ID,
		DisplayTaskName: displayTaskInfo.Name,
		Execution:       t.Execution,
		Requester:       t.Requester,
		Mainline:        !t.IsPatchRequest(),
	}
}

func convertToParquet(results []testresult.TestResult, info testresult.TestResultsInfo, createdAt time.Time) *testresult.ParquetTestResults {
	convertedResults := make([]testresult.ParquetTestResult, len(results))
	for i, result := range results {
		convertedResults[i] = createParquetTestResult(result)
	}

	parquetResults := &testresult.ParquetTestResults{
		Version:   info.Version,
		Variant:   info.Variant,
		TaskName:  info.TaskName,
		TaskID:    info.TaskID,
		Execution: int32(info.Execution),
		Requester: info.Requester,
		CreatedAt: createdAt.UTC(),
		Results:   convertedResults,
	}
	if info.DisplayTaskName != "" {
		parquetResults.DisplayTaskName = utility.ToStringPtr(info.DisplayTaskName)
	}
	if info.DisplayTaskID != "" {
		parquetResults.DisplayTaskID = utility.ToStringPtr(info.DisplayTaskID)
	}
	return parquetResults
}

func createParquetTestResult(t testresult.TestResult) testresult.ParquetTestResult {
	result := testresult.ParquetTestResult{
		TestName:       t.TestName,
		Status:         t.Status,
		LogInfo:        t.LogInfo,
		TaskCreateTime: t.TaskCreateTime.UTC(),
		TestStartTime:  t.TestStartTime.UTC(),
		TestEndTime:    t.TestEndTime.UTC(),
	}
	if t.DisplayTestName != "" {
		result.DisplayTestName = utility.ToStringPtr(t.DisplayTestName)
	}
	if t.GroupID != "" {
		result.GroupID = utility.ToStringPtr(t.GroupID)
	}
	if t.LogTestName != "" {
		result.LogTestName = utility.ToStringPtr(t.LogTestName)
	}
	if t.LogURL != "" {
		result.LogURL = utility.ToStringPtr(t.LogURL)
	}
	if t.RawLogURL != "" {
		result.RawLogURL = utility.ToStringPtr(t.RawLogURL)
	}
	if t.LogTestName != "" || t.LogURL != "" || t.RawLogURL != "" {
		result.LineNum = utility.ToInt32Ptr(int32(t.LineNum))
	}
	return result
}

func makeTestResults(t *task.Task, results []testresult.TestResult) []testresult.TestResult {
	var newResults []testresult.TestResult
	for _, r := range results {
		if r.DisplayTestName == "" {
			r.DisplayTestName = r.TestName
		}
		var logInfo *testresult.TestLogInfo
		if r.LogInfo != nil {
			logInfo = &testresult.TestLogInfo{
				LogName:       r.LogInfo.LogName,
				LineNum:       r.LogInfo.LineNum,
				RenderingType: r.LogInfo.RenderingType,
				Version:       r.LogInfo.Version,
			}
			logInfo.LogsToMerge = append(logInfo.LogsToMerge, r.LogInfo.LogsToMerge...)
		}

		newResults = append(newResults, testresult.TestResult{
			TestName:        utility.RandomString(),
			DisplayTestName: r.DisplayTestName,
			Status:          r.Status,
			LogInfo:         logInfo,
			GroupID:         r.GroupID,
			LogURL:          r.LogURL,
			RawLogURL:       r.RawLogURL,
			LineNum:         r.LineNum,
			TaskCreateTime:  t.CreateTime,
			TestStartTime:   r.TestStartTime,
			TestEndTime:     r.TestEndTime,
		})
	}

	return newResults
}
