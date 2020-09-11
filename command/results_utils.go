package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/pkg/errors"
)

// sendResults sends the specified test results to the API server. For test
// results that use raw logs, it also sends the test logs.
func sendResults(ctx context.Context, conf *model.TaskConfig,
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

	err := comm.SendTestResults(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, results)
	if err != nil {
		return errors.WithStack(err)
	}

	logger.Task().Info("Attach test results succeeded")

	return nil
}
