package command

import (
	"context"
	"io/ioutil"
	"path"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
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
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if len(c.Files) == 0 {
		return errors.Errorf("error validating params: must specify at least one "+
			"file pattern to parse: '%+v'", params)
	}
	return nil
}

func (c *goTest2JSONCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "failed to expand files")
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
	logger.Task().Infof("Parsing test file '%s'...", file)
	results, err := c.loadJSONFile(file, logger, conf)
	if err != nil {
		logger.Task().Errorf("Error parsing test file: %s", err)
		return errors.Wrapf(err, "Error parsing test file: %s", err)
	}

	if len(results.Tests) == 0 {
		logger.Task().Warning("Parsed no events from test file")
		if len(results.Log) == 0 {
			logger.Task().Warning("Test log is empty")
			return nil
		}
	}

	logger.Task().Info("Sending test logs to server...")
	_, suiteName := filepath.Split(file)
	log := model.TestLog{
		Name:          suiteName,
		Task:          conf.Task.Id,
		TaskExecution: conf.Task.Execution,
		Lines:         results.Log,
	}
	logId, err := sendTestLog(ctx, comm, conf, &log)
	if err != nil {
		logger.Task().Errorf("failed to post log: %v", err)
		return errors.Wrap(err, "failed to post log")
	}
	logger.Task().Info("Finished posting logs to server")

	if len(results.Tests) == 0 {
		return nil
	}

	// exclude package level results if we have more than 1 test
	if len(results.Tests) > 1 {
		key := test2json.TestKey{
			Name:      "",
			Iteration: 0,
		}
		delete(results.Tests, key)
	}

	evgResults := make([]task.TestResult, 0, len(results.Tests))
	for _, v := range results.Tests {
		testResult := goTest2JSONToTestResult(suiteName, v.Name, conf.Task, v)
		testResult.LogId = logId
		evgResults = append(evgResults, testResult)
	}

	logger.Task().Info("Sending parsed results to server...")
	if err := sendTestResults(ctx, comm, logger, conf, &task.LocalTestResults{
		Results: evgResults,
	}); err != nil {
		logger.Task().Errorf("problem posting parsed results to the server: %+v", err)
		return errors.Wrap(err, "problem sending test results")
	}
	logger.Task().Info("Successfully sent parsed results to server")

	return nil
}

func (c *goTest2JSONCommand) loadJSONFile(file string, logger client.LoggerProducer, conf *internal.TaskConfig) (*test2json.TestResults, error) {
	filePath := file
	if !path.IsAbs(filePath) {
		filePath = path.Join(conf.WorkDir, filePath)
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.Task().Errorf("Failed to open '%s'", filePath)
		return nil, errors.Wrapf(err, "failed to open: %s", filePath)
	}

	results, err := test2json.ProcessBytes(data)
	if err != nil {
		logger.Task().Errorf("Failed to process '%s': %+v", filePath, err)
		return nil, errors.Wrapf(err, "failed to process '%s'", filePath)
	}

	return results, nil
}

func goTest2JSONToTestResult(suiteName, key string, t *task.Task, test *test2json.Test) task.TestResult {
	result := task.TestResult{
		TestFile:    key,
		LogTestName: suiteName,
		LineNum:     test.FirstLogLine,
		Status:      evergreen.TestFailedStatus,
		StartTime:   float64(test.StartTime.Unix()),
		EndTime:     float64(test.EndTime.Unix()),
	}
	switch test.Status {
	case test2json.Passed:
		result.Status = evergreen.TestSucceededStatus
	case test2json.Skipped:
		result.Status = evergreen.TestSkippedStatus
	}

	return result
}
