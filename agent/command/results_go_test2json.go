package command

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/go-test2json"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type goTest2JSONCommand struct {
	Files []string `mapstructure:"files" plugin:"expand"`

	base
}

func goTest2JSONFactory() Command          { return &goTest2JSONCommand{} }
func (c *goTest2JSONCommand) Name() string { return "gotest.parse_json" }

func (c *goTest2JSONCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "decoding mapstructure params")
	}

	if len(c.Files) == 0 {
		return errors.Errorf("must specify at least one file pattern to parse")
	}
	return nil
}

func (c *goTest2JSONCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// All file patterns should be relative to the task's working directory.
	for i, file := range c.Files {
		c.Files[i] = filepath.Join(conf.WorkDir, file)
	}

	files, err := globFiles(c.Files...)
	if err != nil {
		return errors.Wrapf(err, "obtaining names of output files")
	}
	catcher := grip.NewBasicCatcher()
	for _, file := range files {
		catcher.Add(c.executeOneFile(ctx, file, comm, logger, conf))
	}

	return catcher.Resolve()
}

func (c *goTest2JSONCommand) executeOneFile(ctx context.Context, file string,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	logger.Task().Infof("parsing test file '%s'...", file)
	results, err := c.loadJSONFile(file, logger, conf)
	if err != nil {
		return errors.Wrapf(err, "parsing JSON test file")
	}

	if len(results.Tests) == 0 {
		logger.Task().Warning("Parsed no tests from test file.")
		if len(results.Log) == 0 {
			logger.Task().Warning("Test log is empty.")
			return nil
		}
	}

	logger.Task().Info("Posting test logs...")
	_, suiteName := filepath.Split(file)
	log := model.TestLog{
		Name:          suiteName,
		Task:          conf.Task.Id,
		TaskExecution: conf.Task.Execution,
		Lines:         results.Log,
	}
	if err := sendTestLog(ctx, comm, conf, &log); err != nil {
		return errors.Wrap(err, "sending test log")
	}
	logger.Task().Info("Successfully posted test logs.")

	if len(results.Tests) == 0 {
		return nil
	}

	// Exclude package level results if we have more than 1 test.
	if len(results.Tests) > 1 {
		key := test2json.TestKey{
			Name:      "",
			Iteration: 0,
		}
		delete(results.Tests, key)
	}

	evgResults := make([]testresult.TestResult, 0, len(results.Tests))
	for _, v := range results.Tests {
		evgResults = append(evgResults, goTest2JSONToTestResult(suiteName, v.Name, conf.Task, v))
	}

	return errors.Wrap(sendTestResults(ctx, comm, logger, conf, evgResults), "sending test results")
}

func (c *goTest2JSONCommand) loadJSONFile(file string, logger client.LoggerProducer, conf *internal.TaskConfig) (*test2json.TestResults, error) {
	filePath := file
	if !path.IsAbs(filePath) {
		filePath = path.Join(conf.WorkDir, filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading file '%s'", filePath)
	}

	results, err := test2json.ProcessBytes(data)
	if err != nil {
		return nil, errors.Wrapf(err, "processing file '%s'", filePath)
	}

	return results, nil
}

func goTest2JSONToTestResult(suiteName, key string, t *task.Task, test *test2json.Test) testresult.TestResult {
	result := testresult.TestResult{
		TestName:      key,
		LogTestName:   suiteName,
		LineNum:       test.FirstLogLine,
		Status:        evergreen.TestFailedStatus,
		TestStartTime: test.StartTime,
		TestEndTime:   test.EndTime,
	}
	switch test.Status {
	case test2json.Passed:
		result.Status = evergreen.TestSucceededStatus
	case test2json.Skipped:
		result.Status = evergreen.TestSkippedStatus
	}

	return result
}
