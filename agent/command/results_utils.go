package command

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
)

// testResultParseOptions describes the task run that parsed test results belong
// to and the directory that result file paths are resolved against. It is the
// minimal information the test result parsers need, so results can be parsed on
// behalf of a task other than the one that is currently running (e.g. a virtual
// task).
type testResultParseOptions struct {
	WorkDir       string
	TaskID        string
	TaskExecution int
}

// parseOptionsForTask returns the parse options for the currently running task.
func parseOptionsForTask(conf *internal.TaskConfig) testResultParseOptions {
	return testResultParseOptions{
		WorkDir:       conf.WorkDir,
		TaskID:        conf.Task.Id,
		TaskExecution: conf.Task.Execution,
	}
}

// sendTestResults sends the test results to the backend results service.
func sendTestResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, results []testresult.TestResult) error {
	if len(results) == 0 {
		return errors.New("cannot send nil results")
	}

	logger.Task().Info(ctx, "Attaching test results...")
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := attachTestResults(ctx, conf, td, comm, results); err != nil {
		return errors.Wrap(err, "sending test results")
	}

	logger.Task().Info(ctx, "Successfully attached results.")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to backend
// logging and results services. Test logs are uploaded in parallel using a
// worker pool for improved performance.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, logs []testlog.TestLog, results []testresult.TestResult) error {
	if len(logs) == 0 {
		return sendTestResults(ctx, comm, logger, conf, results)
	}

	logger.Task().Info(ctx, "Posting test logs...")

	opts := redactor.RedactionOptions{
		Expansions:         conf.NewExpansions,
		Redacted:           conf.Redacted,
		InternalRedactions: conf.InternalRedactions,
	}

	succeeded, err := agentutil.ParallelWorkerExec(ctx, "sending test log", logs, logger.Task(),
		func(log *testlog.TestLog) error {
			return taskoutput.AppendTestLog(ctx, &conf.Task, opts, log, conf.S3Usage)
		},
	)
	if err != nil {
		return err
	}

	logger.Task().Infof(ctx, "Finished posting test logs (%d of %d succeeded).", succeeded, len(logs))

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

const (
	maxTestResultsInterval   = 24 * time.Hour
	failedTestsSampleSize    = 10
	maxDisplayTestNameLength = 256
)

func uploadTestResults(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, results []testresult.TestResult, td client.TaskData, output *task.TaskOutput) (bool, error) {
	createdAt := conf.TestResultsCreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
		conf.TestResultsCreatedAt = createdAt
	}
	if time.Since(conf.TestResultsCreatedAt) > maxTestResultsInterval {
		err := errors.Errorf("Cannot append test results more than %s after the first upload. Consider uploading all test results at the end of the task. (DEVPROD-32331)", maxTestResultsInterval)
		grip.Alert(ctx, err)
		return false, err
	}
	info := makeTestResultsInfo(conf.Task, conf.DisplayTaskInfo)
	newResults := makeTestResults(&conf.Task, results)

	// A task run's test results can be attached by multiple commands, all of
	// which must land in the same parquet object, so append to any results
	// already uploaded for this run.
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

	if err = uploadTestResultsParquet(ctx, conf.TaskOutput, *output, info, createdAt, allResults); err != nil {
		return false, errors.Wrap(err, "uploading parquet test results")
	}

	failedCount, failedTests := computeTestResultsStats(allResults)
	tr.Stats = testresult.TaskTestResultsStats{
		FailedCount: failedCount,
		TotalCount:  len(allResults),
	}
	tr.FailedTestsSample = failedTests

	if err = comm.SendTestResults(ctx, td, tr); err != nil {
		return false, errors.Wrap(err, "sending test results")
	}
	return tr.Stats.FailedCount > 0, nil
}

// uploadTestResultsParquet writes the test results to the parquet object for the
// task run described by info.
func uploadTestResultsParquet(ctx context.Context, credentials evergreen.S3Credentials, output task.TaskOutput, info testresult.TestResultsInfo, createdAt time.Time, results []testresult.TestResult) error {
	bucket, err := output.TestResults.GetBucket(ctx, credentials)
	if err != nil {
		return err
	}
	w, err := bucket.Writer(ctx, testresult.PartitionKey(createdAt, info.Project, info.ID()))
	if err != nil {
		return errors.Wrap(err, "creating Presto bucket writer")
	}
	defer w.Close()

	return errors.Wrap(parquet.Write(w, []testresult.ParquetTestResults{*convertToParquet(results, info, createdAt)}), "writing Parquet test results")
}

// computeTestResultsStats returns the number of failed tests and a sample of
// their display names.
func computeTestResultsStats(results []testresult.TestResult) (failedCount int, failedSample []string) {
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			if len(failedSample) < failedTestsSampleSize {
				failedSample = append(failedSample, result.GetDisplayTestName())
			}
			failedCount++
		}
	}
	return failedCount, failedSample
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
		if len(r.DisplayTestName) > maxDisplayTestNameLength {
			r.DisplayTestName = strings.ToValidUTF8(r.DisplayTestName[:maxDisplayTestNameLength], "")
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
