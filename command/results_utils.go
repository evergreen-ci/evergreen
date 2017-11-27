package command

// sendJSONResults is responsible for sending the
import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/pkg/errors"
)

// specified file to the API Server
func sendJSONResults(ctx context.Context, conf *model.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator,
	results *task.LocalTestResults) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	for i, res := range results.Results {
		if ctx.Err() != nil {
			return errors.Errorf("operation canceled after uploading ")
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

	err := comm.SendTestResults(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, results)
	if err != nil {
		return errors.WithStack(err)
	}

	logger.Task().Info("Attach test results succeeded")

	return nil
}

// sendJSONLogs is responsible for sending the specified logs
// to the API Server. If successful, it returns a log ID that can be used
// to refer to the log object in test results.
func sendJSONLogs(ctx context.Context, logger client.LoggerProducer,
	comm client.Communicator, td client.TaskData, logs *model.TestLog) (string, error) {

	logger.Execution().Infof("Attaching test logs for %s", logs.Name)
	logID, err := comm.SendTestLog(ctx, td, logs)
	if err != nil {
		return "", errors.WithStack(err)
	}

	logger.Task().Info("Attach test logs succeeded")
	return logID, nil
}
