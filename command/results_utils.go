package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// sendTestResults sends the test results to the API server. For test results
// that use raw logs, it also sends the test logs.
func sendTestResults(ctx context.Context, conf *model.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator,
	results *task.LocalTestResults) error {

	if results == nil || len(results.Results) == 0 {
		return errors.New("cannot send nil results")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	for i, res := range results.Results {
		if ctx.Err() != nil {
			return errors.Errorf("operation canceled during uploading")
		}

		if res.LogRaw != "" {
			logger.Execution().Info("Attaching raw test logs")
			testLogs := &model.TestLog{
				Name:          res.TestFile,
				Task:          conf.Task.Id,
				TaskExecution: conf.Task.Execution,
				Lines:         []string{res.LogRaw},
			}

			id, err := comm.SendTestLog(ctx, td, testLogs)
			if err != nil {
				logger.Execution().Errorf("problem posting raw logs from results %s", err.Error())
			} else {
				results.Results[i].LogId = id
			}

			// clear the logs from the TestResult struct after it has been saved in the test logs. Since they are
			// being saved in the test_logs collection, we can clear them to prevent them from being saved in the task
			// collection.
			results.Results[i].LogRaw = ""
		}
	}
	logger.Execution().Info("attaching test results")

	if err := comm.SendTestResults(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, results); err != nil {
		return errors.WithStack(err)
	}

	// kim: TODO: have to start up Cedar mock server for this to run properly in
	// tests.
	// if err := sendTestResultsToCedar(ctx, conf.Task, comm, results); err != nil {
	//     return errors.Wrap(err, "sending test results to Cedar")
	// }

	logger.Task().Info("Attach test results succeeded")

	return nil
}

// sendTestLogsAndResults sends the test logs and test results to the API
// server.
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

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId

			allResults.Results = append(allResults.Results, result)
		}
	}
	logger.Task().Info("Finished posting logs to server")

	// ship the parsed results off to the server
	logger.Task().Info("Sending parsed results to server...")

	if err := comm.SendTestResults(ctx, td, &allResults); err != nil {
		logger.Task().Errorf("problem posting parsed results to the server: %+v", err)
		return errors.Wrap(err, "problem sending test results")
	}
	// if err := sendTestResultsToCedar(ctx, conf.Task, comm, &allResults); err != nil {
	//     return errors.Wrap(err, "sending test results to Cedar")
	// }

	logger.Task().Info("Successfully sent parsed results to server")

	return nil
}

// sendTestResultsToCedar sends the given test results to Cedar.
func sendTestResultsToCedar(ctx context.Context, t *task.Task, comm client.Communicator, results *task.LocalTestResults) error {
	conn, err := comm.GetCedarGRPCConn(ctx)
	if err != nil {
		return errors.Wrap(err, "getting cedar connection")
	}
	client, err := testresults.NewClientWithExistingConnection(ctx, conn)
	if err != nil {
		return errors.Wrap(err, "creating test results client")
	}

	id, err := client.CreateRecord(ctx, testresults.CreateOptions{
		Project:     t.Project,
		Version:     t.Version,
		Variant:     t.BuildVariant,
		TaskID:      t.Id,
		TaskName:    t.DisplayName,
		Execution:   int32(t.Execution),
		RequestType: t.Requester,
		Mainline:    !t.IsPatchRequest(),
	})
	if err != nil {
		return errors.Wrap(err, "creating test results record")
	}

	rs := makeCedarTestResults(id, t, results)
	if err = client.AddResults(ctx, rs); err != nil {
		return errors.Wrap(err, "adding test results")
	}

	if err = client.CloseRecord(ctx, id); err != nil {
		return errors.Wrap(err, "closing test results record")
	}

	return nil
}

func makeCedarTestResults(id string, t *task.Task, results *task.LocalTestResults) testresults.Results {
	var rs testresults.Results
	for _, r := range results.Results {
		rs.Results = append(rs.Results, testresults.Result{
			// Name:        r.Name, // kim: TODO: figure out test name
			Trial:       int32(r.Execution), // kim: TODO: confirm that these are equivalent,
			Status:      r.Status,
			LogURL:      r.URL, // kim: TODO: figure out if this is supposed to be URL or URLRaw
			LineNum:     int32(r.LineNum),
			TaskCreated: t.CreateTime,
			// kim: TODO: figure out what format the test times are in since they're untyped floats.
			// TestStarted  : (r.StartTime),
			// TestEnded    : (r.EndTime),
		})
	}
	return rs
}
