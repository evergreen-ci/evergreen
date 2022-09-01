package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
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
func (c *attachResults) Name() string { return evergreen.AttachResultsCommandName }

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
		reportFileLoc = getJoinedWithWorkDir(conf, c.FileLoc)
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
		return errors.Wrap(err, "sending test logs")
	}

	return sendTestResults(ctx, comm, logger, conf, results)
}

func (c *attachResults) sendTestLogs(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, comm client.Communicator, results *task.LocalTestResults) error {
	logger.Execution().Info("posting test logs...")
	for i, res := range results.Results {
		if ctx.Err() != nil {
			return errors.Errorf("operation canceled")
		}

		if res.LogRaw != "" {
			testLogs := &model.TestLog{
				// When sending test logs to Cedar we need to
				// use a unique string since there may be
				// duplicate file names if there are duplicate
				// test names.
				Name:          utility.RandomString(),
				Task:          conf.Task.Id,
				TaskExecution: conf.Task.Execution,
				Lines:         strings.Split(res.LogRaw, "\n"),
			}

			if err := sendTestLog(ctx, comm, conf, testLogs); err != nil {
				// Continue on error to let other logs be
				// posted.
				logger.Execution().Errorf("error posting test logs: %+v", err)
			} else {
				results.Results[i].LogTestName = testLogs.Name
			}
		}
	}
	logger.Execution().Info("finished posted test logs")

	return nil
}
