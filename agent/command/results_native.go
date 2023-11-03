package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type nativeTestResults struct {
	Results []nativeTestResult `json:"results"`
}

func (t nativeTestResults) convertToService() []testresult.TestResult {
	serviceResults := make([]testresult.TestResult, len(t.Results))
	for i, result := range t.Results {
		serviceResults[i] = result.convertToService()
	}

	return serviceResults
}

type nativeTestResult struct {
	TaskID    string  `json:"task_id"`
	Execution int     `json:"execution"`
	TestFile  string  `json:"test_file"`
	Status    string  `json:"status"`
	GroupID   string  `json:"group_id"`
	LogRaw    string  `json:"log_raw"`
	URL       string  `json:"url"`
	URLRaw    string  `json:"url_raw"`
	LineNum   int     `json:"line_num"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`

	// logTestName is not part of the command API and used for internal
	// purposes only.
	logTestName string `json:"-"`
}

func (t nativeTestResult) convertToService() testresult.TestResult {
	return testresult.TestResult{
		TaskID:        t.TaskID,
		Execution:     t.Execution,
		TestName:      t.TestFile,
		GroupID:       t.GroupID,
		Status:        t.Status,
		LogTestName:   t.logTestName,
		LogURL:        t.URL,
		RawLogURL:     t.URLRaw,
		LineNum:       t.LineNum,
		TestStartTime: utility.FromPythonTime(t.Start),
		TestEndTime:   utility.FromPythonTime(t.End),
	}
}

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
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.FileLoc == "" {
		return errors.New("file location cannot be blank")
	}

	return nil
}

func (c *attachResults) expandAttachResultsParams(taskConfig *internal.TaskConfig) error {
	var err error

	c.FileLoc, err = taskConfig.Expansions.ExpandString(c.FileLoc)
	if err != nil {
		return errors.Wrap(err, "expanding file location")
	}

	return nil
}

// Execute carries out the attachResults command - this is required
// to satisfy the 'Command' interface
func (c *attachResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := c.expandAttachResultsParams(conf); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	reportFileLoc := c.FileLoc
	if !filepath.IsAbs(c.FileLoc) {
		reportFileLoc = getWorkingDirectory(conf, c.FileLoc)
	}

	// attempt to open the file
	reportFile, err := os.Open(reportFileLoc)
	if err != nil {
		return errors.Wrapf(err, "opening report file '%s'", reportFileLoc)
	}
	defer reportFile.Close()

	var nativeResults nativeTestResults
	if err = utility.ReadJSON(reportFile, &nativeResults); err != nil {
		return errors.Wrapf(err, "reading report file '%s'", reportFileLoc)
	}

	if err := c.sendTestLogs(ctx, conf, logger, comm, &nativeResults); err != nil {
		return errors.Wrap(err, "sending test logs")
	}

	return sendTestResults(ctx, comm, logger, conf, nativeResults.convertToService())
}

func (c *attachResults) sendTestLogs(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, comm client.Communicator, results *nativeTestResults) error {
	logger.Execution().Info("Posting test logs...")
	for i, res := range results.Results {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "operation canceled")
		}

		if res.LogRaw != "" {
			testLogs := &testlog.TestLog{
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
				logger.Execution().Error(errors.Wrap(err, "sending test logs"))
			} else {
				results.Results[i].logTestName = testLogs.Name
			}
		}
	}
	logger.Execution().Info("finished posted test logs")

	return nil
}
