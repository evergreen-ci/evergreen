package command

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// sendTestResults sends the test results to the API server and Cedar.
func sendTestResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, results *task.LocalTestResults) error {
	if results == nil || len(results.Results) == 0 {
		return errors.New("cannot send nil results")
	}

	logger.Task().Info("Attaching results to server...")
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if conf.ProjectRef.IsCedarTestResultsEnabled() {
		if err := sendTestResultsToCedar(ctx, conf, td, comm, results); err != nil {
			logger.Task().Errorf("problem posting parsed results to the cedar: %+v", err)
			return errors.Wrap(err, "problem sending test results to cedar")
		}
	} else {
		if err := comm.SendTestResults(ctx, td, results); err != nil {
			logger.Task().Errorf("problem posting parsed results to evergreen: %+v", err)
			return errors.Wrap(err, "problem sending test results to evergreen")
		}
	}
	logger.Task().Info("Successfully attached results to server")

	return nil
}

// sendTestLog sends test logs to the API server and Cedar.
func sendTestLog(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, log *model.TestLog) (string, error) {
	if conf.ProjectRef.IsCedarTestResultsEnabled() {
		return "", errors.Wrap(sendTestLogToCedar(ctx, conf.Task, comm, log), "problem sending test logs to cedar")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	logId, err := comm.SendTestLog(ctx, td, log)
	return logId, errors.Wrap(err, "problem sending test logs to evergreen")
}

// sendTestLogsAndResults sends the test logs and test results to the API
// server and Cedar.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, logs []model.TestLog, results [][]task.TestResult) error {
	// ship all of the test logs off to the server
	logger.Task().Info("Sending test logs to server...")
	allResults := task.LocalTestResults{}
	for idx, log := range logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		logId, err := sendTestLog(ctx, comm, conf, &log)
		if err != nil {
			// continue on error to let the other logs be posted
			logger.Task().Errorf("problem posting log: %v", err)
		}

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId

			allResults.Results = append(allResults.Results, result)
		}
	}
	logger.Task().Info("Finished posting logs to server")

	return sendTestResults(ctx, comm, logger, conf, &allResults)
}

func sendTestResultsToCedar(ctx context.Context, conf *internal.TaskConfig, td client.TaskData, comm client.Communicator, results *task.LocalTestResults) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "getting cedar connection")
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

	cedarResults, failed := makeCedarTestResults(conf.CedarTestResultsID, conf.Task, results)
	if err = client.AddResults(ctx, cedarResults); err != nil {
		return errors.Wrap(err, "adding test results")
	}

	if err = client.CloseRecord(ctx, conf.CedarTestResultsID); err != nil {
		return errors.Wrap(err, "closing test results record")
	}

	if err = comm.SetHasCedarResults(ctx, td, failed); err != nil {
		return errors.Wrap(err, "problem setting HasCedarResults flag in task")
	}

	return nil
}

func sendTestLogToCedar(ctx context.Context, t *task.Task, comm client.Communicator, log *model.TestLog) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "problem setting up cedar grpc connection for test %s", log.Name)
	}

	timberOpts := &buildlogger.LoggerOptions{
		Project:    t.Project,
		Version:    t.Version,
		Variant:    t.BuildVariant,
		TaskName:   t.DisplayName,
		TaskID:     t.Id,
		Execution:  int32(t.Execution),
		TestName:   log.Name,
		Mainline:   !t.IsPatchRequest(),
		Storage:    buildlogger.LogStorageS3,
		ClientConn: conn,
	}
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	sender, err := buildlogger.NewLoggerWithContext(ctx, log.Name, levelInfo, timberOpts)
	if err != nil {
		return errors.Wrapf(err, "error creating buildlogger logger for test result %s", log.Name)
	}

	sender.Send(message.ConvertToComposer(level.Info, strings.Join(log.Lines, "\n")))
	if err = sender.Close(); err != nil {
		return errors.Wrapf(err, "error closing buildlogger logger for test result %s", log.Name)
	}

	return nil
}

func makeCedarTestResultsRecord(conf *internal.TaskConfig, displayTaskInfo *apimodels.DisplayTaskInfo) testresults.CreateOptions {
	return testresults.CreateOptions{
		Project:                conf.Task.Project,
		Version:                conf.Task.Version,
		Variant:                conf.Task.BuildVariant,
		TaskID:                 conf.Task.Id,
		TaskName:               conf.Task.DisplayName,
		DisplayTaskID:          displayTaskInfo.ID,
		DisplayTaskName:        displayTaskInfo.Name,
		Execution:              int32(conf.Task.Execution),
		RequestType:            conf.Task.Requester,
		Mainline:               !conf.Task.IsPatchRequest(),
		HistoricalDataIgnore:   conf.ProjectRef.FilesIgnoredFromCache,
		HistoricalDataDisabled: conf.ProjectRef.IsStatsCacheDisabled(),
	}
}

func makeCedarTestResults(id string, t *task.Task, results *task.LocalTestResults) (testresults.Results, bool) {
	rs := testresults.Results{ID: id}
	failed := false
	for _, r := range results.Results {
		if r.DisplayTestName == "" {
			r.DisplayTestName = r.TestFile
		}
		if r.LogTestName == "" {
			r.LogTestName = r.TestFile
		}
		rs.Results = append(rs.Results, testresults.Result{
			TestName:        utility.RandomString(),
			DisplayTestName: r.DisplayTestName,
			GroupID:         r.GroupID,
			Status:          r.Status,
			LogTestName:     r.LogTestName,
			LogURL:          r.URL,
			RawLogURL:       r.URLRaw,
			LineNum:         int32(r.LineNum),
			TaskCreated:     t.CreateTime,
			TestStarted:     time.Unix(int64(r.StartTime), 0),
			TestEnded:       time.Unix(int64(r.EndTime), 0),
		})

		if r.Status == evergreen.TestFailedStatus {
			failed = true
		}
	}

	return rs, failed
}
