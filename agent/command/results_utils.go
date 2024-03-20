package command

import (
	"context"

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
	"github.com/pkg/errors"
)

// sendTestResults sends the test results to the backend results service.
func sendTestResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, results []testresult.TestResult) error {
	if len(results) == 0 {
		return errors.New("cannot send nil results")
	}

	logger.Task().Info("Attaching test results...")
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := sendTestResultsToCedar(ctx, conf, td, comm, results); err != nil {
		return errors.Wrap(err, "sending test results to Cedar")
	}

	logger.Task().Info("Successfully attached results.")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to backend
// logging results services.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, logs []testlog.TestLog, results [][]testresult.TestResult) error {
	logger.Task().Info("Posting test logs...")
	var allResults []testresult.TestResult
	for idx, log := range logs {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while sending test logs")
		}

		if err := taskoutput.AppendTestLog(ctx, comm, &conf.Task, redactor.RedactionOptions{Expansions: conf.NewExpansions, Redacted: conf.Redacted}, &log); err != nil {
			// Continue on error to let the other logs be posted.
			logger.Task().Error(errors.Wrap(err, "sending test log"))
		}

		// Add all of the test results that correspond to that log to
		// the full list of results.
		allResults = append(allResults, results[idx]...)
	}
	logger.Task().Info("Finished posting test logs.")

	return sendTestResults(ctx, comm, logger, conf, allResults)
}

func sendTestResultsToCedar(ctx context.Context, conf *internal.TaskConfig, td client.TaskData, comm client.Communicator, results []testresult.TestResult) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "getting Cedar connection")
	}
	client, err := testresults.NewClientWithExistingConnection(ctx, conn)
	if err != nil {
		return errors.Wrap(err, "creating test results client")
	}
	displayTaskInfo, err := comm.GetDisplayTaskInfoFromExecution(ctx, td)
	if err != nil {
		return errors.Wrap(err, "getting this task's display task info")
	}

	if conf.CedarTestResultsID == "" {
		conf.CedarTestResultsID, err = client.CreateRecord(ctx, makeCedarTestResultsRecord(conf, displayTaskInfo))
		if err != nil {
			return errors.Wrap(err, "creating test results record")
		}
	}

	cedarResults, failed := makeCedarTestResults(conf.CedarTestResultsID, &conf.Task, results)
	if err = client.AddResults(ctx, cedarResults); err != nil {
		return errors.Wrap(err, "adding test results")
	}

	if err = client.CloseRecord(ctx, conf.CedarTestResultsID); err != nil {
		return errors.Wrap(err, "closing test results record")
	}

	if err := comm.SetResultsInfo(ctx, td, testresult.TestResultsServiceCedar, failed); err != nil {
		return errors.Wrap(err, "setting results info in the task")
	}

	return nil
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
