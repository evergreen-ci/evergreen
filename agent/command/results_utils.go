package command

import (
	"context"
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
	"github.com/evergreen-ci/timber/testresults"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
		return errors.Wrap(err, "sending test results to Cedar")
	}

	logger.Task().Info("Successfully attached results.")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to backend
// logging and results services.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, logs []testlog.TestLog, results []testresult.TestResult) error {
	logger.Task().Info("Posting test logs...")
	for _, log := range logs {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while sending test logs")
		}

		if err := taskoutput.AppendTestLog(ctx, &conf.Task, redactor.RedactionOptions{
			Expansions:         conf.NewExpansions,
			Redacted:           conf.Redacted,
			InternalRedactions: conf.InternalRedactions,
		}, &log); err != nil {
			// Continue on error to let the other logs get posted.
			logger.Task().Error(errors.Wrap(err, "sending test log"))
		}
	}
	logger.Task().Info("Finished posting test logs.")

	return sendTestResults(ctx, comm, logger, conf, results)
}

func attachTestResults(ctx context.Context, conf *internal.TaskConfig, td client.TaskData, comm client.Communicator, results []testresult.TestResult) error {
	var failed bool
	var err error
	output, ok := conf.Task.GetTaskOutputSafe()
	if !ok || output.TestResults.Version == 0 {
		failed, err = attachCedar(ctx, comm, conf, results)
		if err != nil {
			return errors.Wrap(err, "attaching test results to Cedar")
		}
	} else {
		failed, err = attach(ctx, comm, conf, results, td, output)
		if err != nil {
			return errors.Wrap(err, "attaching test results")
		}
	}
	conf.HasTestResults = true
	if err := comm.SetResultsInfo(ctx, td, failed); err != nil {
		return errors.Wrap(err, "setting results info in the task")
	}
	if failed {
		conf.HasFailingTestResult = true
	}
	return nil
}

func attachCedar(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, results []testresult.TestResult) (bool, error) {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return false, errors.Wrap(err, "getting Cedar connection")
	}
	client, err := testresults.NewClientWithExistingConnection(ctx, conn)
	if err != nil {
		return false, errors.Wrap(err, "creating test results client")
	}

	if conf.CedarTestResultsID == "" {
		conf.CedarTestResultsID, err = client.CreateRecord(ctx, makeCedarTestResultsRecord(conf, conf.DisplayTaskInfo))
		if err != nil {
			return false, errors.Wrap(err, "creating test results record")
		}
	}

	cedarResults, failed := makeCedarTestResults(conf.CedarTestResultsID, &conf.Task, results)
	if err = client.AddResults(ctx, cedarResults); err != nil {
		return false, errors.Wrap(err, "adding test results")
	}

	if err = client.CloseRecord(ctx, conf.CedarTestResultsID); err != nil {
		return false, errors.Wrap(err, "closing test results record")
	}
	return failed, nil
}

func attach(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, results []testresult.TestResult, td client.TaskData, output *task.TaskOutput) (bool, error) {
	createdAt := time.Now()
	info := makeTestResultsInfo(conf, conf.DisplayTaskInfo)
	allResults, failed := makeTestResults(&conf.Task, results)
	key := testresult.GetPrestoPartitionKey(createdAt, info.Project, info.ID())

	// TODO: DEVPROD-16200 Implement test result service fetch operations
	//allResults, err := output.TestResults.DownloadParquet(ctx, conf.TaskOutput, key)
	//if err != nil && !pail.IsKeyNotFoundError(err) {
	//	return false, errors.Wrap(err, "getting uploaded test results")
	//}
	//allResults = append(allResults, newResults...)

	err := uploadParquet(ctx, conf, convertToParquet(allResults, info, createdAt), key)
	if err != nil {
		return false, errors.Wrap(err, "uploading parquet test results")
	}
	if err = comm.SendTestResults(ctx, td, allResults, info, createdAt); err != nil {
		return false, errors.Wrap(err, "sending test results")
	}
	return failed, nil
}

func makeTestResultsInfo(conf *internal.TaskConfig, displayTaskInfo *apimodels.DisplayTaskInfo) testresult.TestResultsInfo {
	return testresult.TestResultsInfo{
		Project:         conf.Task.Project,
		Version:         conf.Task.Version,
		Variant:         conf.Task.BuildVariant,
		TaskID:          conf.Task.Id,
		TaskName:        conf.Task.DisplayName,
		DisplayTaskID:   displayTaskInfo.ID,
		DisplayTaskName: displayTaskInfo.Name,
		Execution:       conf.Task.Execution,
		RequestType:     conf.Task.Requester,
		Mainline:        !conf.Task.IsPatchRequest(),
	}
}

func convertToParquet(results []testresult.TestResult, info testresult.TestResultsInfo, createdAt time.Time) *testresult.ParquetTestResults {
	convertedResults := make([]testresult.ParquetTestResult, len(results))
	for i, result := range results {
		convertedResults[i] = createParquetTestResult(result)
	}

	parquetResults := &testresult.ParquetTestResults{
		Version:     info.Version,
		Variant:     info.Variant,
		TaskName:    info.TaskName,
		TaskID:      info.TaskID,
		Execution:   int32(info.Execution),
		RequestType: info.RequestType,
		CreatedAt:   createdAt.UTC(),
		Results:     convertedResults,
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

func uploadParquet(ctx context.Context, conf *internal.TaskConfig, results *testresult.ParquetTestResults, key string) error {
	bucket, err := conf.Task.TaskOutputInfo.TestResults.GetPrestoBucket(ctx, conf.TaskOutput)
	if err != nil {
		return err
	}
	w, err := bucket.Writer(ctx, key)
	if err != nil {
		return errors.Wrap(err, "creating Presto bucket writer")
	}
	defer func() {
		err = w.Close()
		grip.WarningWhen(err != nil, message.WrapError(err, message.Fields{
			"message": "closing test results bucket writer",
		}))
	}()

	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(task.ParquetTestResultsSchemaDef)))
	defer func() {
		err = pw.Close()
		grip.WarningWhen(err != nil, message.WrapError(err, message.Fields{
			"message": "closing Parquet test results writer",
		}))
	}()

	return errors.Wrap(pw.Write(results), "writing Parquet test results")
}

func makeTestResults(t *task.Task, results []testresult.TestResult) ([]testresult.TestResult, bool) {
	var newResults []testresult.TestResult
	failed := false
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

		if r.Status == evergreen.TestFailedStatus {
			failed = true
		}
	}

	return newResults, failed
}

func makeCedarTestResultsRecord(conf *internal.TaskConfig, displayTaskInfo *apimodels.DisplayTaskInfo) testresults.CreateOptions {
	return testresults.CreateOptions{
		Project:         conf.Task.Project,
		Version:         conf.Task.Version,
		Variant:         conf.Task.BuildVariant,
		TaskID:          conf.Task.Id,
		TaskName:        conf.Task.DisplayName,
		DisplayTaskID:   displayTaskInfo.ID,
		DisplayTaskName: displayTaskInfo.Name,
		Execution:       int32(conf.Task.Execution),
		RequestType:     conf.Task.Requester,
		Mainline:        !conf.Task.IsPatchRequest(),
	}
}

func makeCedarTestResults(id string, t *task.Task, results []testresult.TestResult) (testresults.Results, bool) {
	rs := testresults.Results{ID: id}
	failed := false
	for _, r := range results {
		if r.DisplayTestName == "" {
			r.DisplayTestName = r.TestName
		}
		var logInfo *testresults.LogInfo
		if r.LogInfo != nil {
			logInfo = &testresults.LogInfo{
				LogName:       r.LogInfo.LogName,
				LineNum:       r.LogInfo.LineNum,
				RenderingType: r.LogInfo.RenderingType,
				Version:       r.LogInfo.Version,
			}
			for _, logName := range r.LogInfo.LogsToMerge {
				logInfo.LogsToMerge = append(logInfo.LogsToMerge, *logName)
			}
		}

		rs.Results = append(rs.Results, testresults.Result{
			TestName:        utility.RandomString(),
			DisplayTestName: r.DisplayTestName,
			Status:          r.Status,
			LogInfo:         logInfo,
			GroupID:         r.GroupID,
			LogURL:          r.LogURL,
			RawLogURL:       r.RawLogURL,
			LineNum:         int32(r.LineNum),
			TaskCreated:     t.CreateTime,
			TestStarted:     r.TestStartTime,
			TestEnded:       r.TestEndTime,
		})

		if r.Status == evergreen.TestFailedStatus {
			failed = true
		}
	}

	return rs, failed
}
