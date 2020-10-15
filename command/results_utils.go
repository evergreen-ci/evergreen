package command

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// sendTestResults sends the test results to the API server and Cedar.
func sendTestResults(ctx context.Context, conf *model.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator,
	results *task.LocalTestResults) error {

	if results == nil || len(results.Results) == 0 {
		return errors.New("cannot send nil results")
	}

	logger.Execution().Info("attaching test results")

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if err := comm.SendTestResults(ctx, td, results); err != nil {
		return errors.WithStack(err)
	}

	// TODO (EVG-7780): send test results for projects that enable it.
	// if err := sendTestResultsToCedar(ctx, conf.Task, td, comm, results); err != nil {
	//     return errors.Wrap(err, "sending test results to Cedar")
	// }

	logger.Task().Info("Attach test results succeeded")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to the API
// server and Cedar.
func sendTestLogsAndResults(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig, logs []model.TestLog, results [][]task.TestResult) error {
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	// ship all of the test logs off to the server
	logger.Task().Info("Sending test logs to server...")

	allResults := task.LocalTestResults{}
	for idx, log := range logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		logId, err := comm.SendTestLog(ctx, td, &log)
		if err != nil {
			// continue on error to let the other logs be posted
			logger.Task().Errorf("problem posting log: %v", err)
		}

		// TODO (EVG-7780): send test results for projects that enable it.
		// if err := sendTestLogToCedar(ctx, td, comm, &log); err != nil {
		// 	logger.Task().Errorf("problem posting test log: %v", err)
		// }

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId

			allResults.Results = append(allResults.Results, result)
		}
	}
	logger.Task().Info("Finished posting logs to server")

	logger.Task().Info("Sending parsed results to server...")

	if err := comm.SendTestResults(ctx, td, &allResults); err != nil {
		logger.Task().Errorf("problem posting parsed results to the server: %+v", err)
		return errors.Wrap(err, "problem sending test results")
	}

	// TODO (EVG-7780): send test results for projects that enable it.
	// if err := sendTestResultsToCedar(ctx, conf.Task, td, comm, &allResults); err != nil {
	//     return errors.Wrap(err, "sending test results to Cedar")
	// }

	logger.Task().Info("Successfully sent parsed results to server")

	return nil
}

func sendTestResultsToCedar(ctx context.Context, t *task.Task, td client.TaskData, comm client.Communicator, results *task.LocalTestResults) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "getting cedar connection")
	}
	client, err := testresults.NewClientWithExistingConnection(ctx, conn)
	if err != nil {
		return errors.Wrap(err, "creating test results client")
	}
	displayTaskName, err := comm.GetDisplayTaskNameFromExecution(ctx, td)
	if err != nil {
		return errors.Wrap(err, "getting this task's display task")
	}

	id, err := client.CreateRecord(ctx, makeCedarTestResultsRecord(t, displayTaskName))
	if err != nil {
		return errors.Wrap(err, "creating test results record")
	}

	if err = client.AddResults(ctx, makeCedarTestResults(id, t, results)); err != nil {
		return errors.Wrap(err, "adding test results")
	}

	if err = client.CloseRecord(ctx, id); err != nil {
		return errors.Wrap(err, "closing test results record")
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

func makeCedarTestResultsRecord(t *task.Task, displayTaskName string) testresults.CreateOptions {
	return testresults.CreateOptions{
		Project:         t.Project,
		Version:         t.Version,
		Variant:         t.BuildVariant,
		TaskID:          t.Id,
		TaskName:        t.DisplayName,
		DisplayTaskName: displayTaskName,
		Execution:       int32(t.Execution),
		RequestType:     t.Requester,
		Mainline:        !t.IsPatchRequest(),
	}
}

func makeCedarTestResults(id string, t *task.Task, results *task.LocalTestResults) testresults.Results {
	rs := testresults.Results{ID: id}
	for _, r := range results.Results {
		rs.Results = append(rs.Results, testresults.Result{
			Name:        r.TestFile,
			Status:      r.Status,
			LineNum:     int32(r.LineNum),
			TaskCreated: t.CreateTime,
			TestStarted: time.Unix(int64(r.StartTime), 0),
			TestEnded:   time.Unix(int64(r.EndTime), 0),
		})
	}
	return rs
}
