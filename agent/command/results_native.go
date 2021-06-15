package command

import (
	"context"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// attachResults is used to attach MCI test results in json
// format to the task page.
type attachResults struct {
	// FileLoc describes the relative path of the file to be sent.
	// Note that this can also be described via expansions.
	FileLoc string `mapstructure:"file_location" plugin:"expand"`
	base
}

func attachResultsFactory() Command   { return &attachResults{} }
func (c *attachResults) Name() string { return "attach.results" }

// ParseParams decodes the S3 push command parameters that are
// specified as part of an AttachPlugin command; this is required
// to satisfy the 'Command' interface
func (c *attachResults) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%v' params", c.Name())
	}

	if c.FileLoc == "" {
		return errors.New("file_location cannot be blank")
	}

	return nil
}

func (c *attachResults) expandAttachResultsParams(taskConfig *internal.TaskConfig) error {
	var err error

	c.FileLoc, err = taskConfig.Expansions.ExpandString(c.FileLoc)
	if err != nil {
		return errors.Wrap(err, "error expanding file_location")
	}

	return nil
}

// Execute carries out the attachResults command - this is required
// to satisfy the 'Command' interface
func (c *attachResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := c.expandAttachResultsParams(conf); err != nil {
		return errors.WithStack(err)
	}

	reportFileLoc := c.FileLoc
	if !filepath.IsAbs(c.FileLoc) {
		reportFileLoc = filepath.Join(conf.WorkDir, c.FileLoc)
	}

	// attempt to open the file
	reportFile, err := os.Open(reportFileLoc)
	if err != nil {
		return errors.Wrapf(err, "couldn't open report file '%s'", reportFileLoc)
	}
	defer reportFile.Close()

	results := &task.LocalTestResults{}
	if err = utility.ReadJSON(reportFile, results); err != nil {
		return errors.Wrapf(err, "couldn't read report file '%s'", reportFileLoc)
	}

	if err := c.sendTestLogs(ctx, conf, logger, comm, results); err != nil {
		return errors.Wrap(err, "problem sending test logs")
	}

	return sendTestResults(ctx, comm, logger, conf, results)
}

func (c *attachResults) sendTestLogs(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, comm client.Communicator, results *task.LocalTestResults) error {
	for i, res := range results.Results {
		if ctx.Err() != nil {
			return errors.Errorf("operation canceled during uploading")
		}

		if res.LogRaw != "" {
			logger.Execution().Info("Attaching raw test logs")
			testLogs := &model.TestLog{
				Task:          conf.Task.Id,
				TaskExecution: conf.Task.Execution,
				Lines:         []string{res.LogRaw},
			}
			if conf.ProjectRef.IsCedarTestResultsEnabled() {
				// When sending test logs to cedar we need to
				// use a unique string since there may be
				// duplicate file names if there are duplicate
				// test names.
				testLogs.Name = utility.RandomString()
			} else {
				testLogs.Name = res.TestFile
			}

			logId, err := sendTestLog(ctx, comm, conf, testLogs)
			if err != nil {
				logger.Execution().Errorf("problem posting raw logs from results %s", err.Error())
			} else {
				results.Results[i].LogId = logId
				results.Results[i].LogTestName = testLogs.Name
			}

			// clear the logs from the TestResult struct after it has been saved in the test logs. Since they are
			// being saved in the test_logs collection, we can clear them to prevent them from being saved in the task
			// collection.
			results.Results[i].LogRaw = ""
		}
	}

	return nil
}
