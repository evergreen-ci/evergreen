package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Golang's JSON result output ('test2json') is a newline delimited list
// of the following struct, encoded as JSON.

// The following rules APPEAR to hold true of Golang's JSON output:
// 1. A single line of output corresponds to a single TestEvent
// 2. Golang offers no field to distinguish between subsequent runs of a test
// such as when setting -test.count to a number greater than 1. However,
// Golang will not run more than one test of the same name at once, even with a
// call to t.Parallel(). This means that you can observe a "fail" or "pass"
// action with the same test name, you know that the next time you see "run",
// it corresponds to a subsequent run.
// 3. Benchmarks do not have a "run" action
// 4. Output has trailing newlines, even on Windows
// 5. test2json's output on Windows uses Unix-style line endings
type goTest2JSONTestEvent struct {
	// Time is the time that the line was processed by by go test
	Time time.Time // encodes as an RFC3339-format string
	// Action is one of:
	// run    - the test has started running
	// pause  - the test has been paused
	// cont   - the test has continued running
	// pass   - the test passed
	// bench  - the benchmark printed log output but did not fail
	// fail   - the test or benchmark failed
	// output - the test printed output
	// Additionally, the following is generated by go 1.10.3 but is undocumented:
	// skip - the test was skipped
	Action string
	// Package optionally specifies the go package path of the test being
	// run
	Package string
	// Test specifies the name of the test function being tested. When
	// go test is reporting benchmarks or package level results, Test is
	// empty
	Test string
	// Elapsed is the number of seconds a test took. It is set only when
	// action is pass or fail
	Elapsed float64 // seconds
	// Output is the line that gotest captured, including trailing newlines
	// or carriage returns. On Windows, the trailing newlines are
	// Unix-style
	Output string
}

type goTest2JSONCommand struct {
	Files []string `mapstructure:"files" plugin:"expand"`

	base
}

type goTest2JSONKey struct {
	name      string
	iteration int
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
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "failed to expand files")
	}

	catcher := grip.NewBasicCatcher()
	for i := range c.Files {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		catcher.Add(c.executeOneFile(ctx, c.Files[i], comm, logger, conf))
	}

	return catcher.Resolve()
}

func (c *goTest2JSONCommand) executeOneFile(ctx context.Context, file string,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	logger.Task().Infof("Parsing test file '%s'...", file)
	events, err := c.loadJSONFile(file, logger, conf)
	if err != nil {
		logger.Task().Errorf("Error parsing test file: %s", err)
		return errors.Wrapf(err, "Error parsing test file: %s", err)
	}

	merged := processParsedJSONFile(events)
	if len(merged) == 0 {
		logger.Task().Warning("Parsed no events from test file")
		return nil
	}

	logger.Task().Info("Sending test logs to server...")
	results := []task.TestResult{}
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	for k, v := range merged {
		testResult, log := goMergedTest2JSONToTestResult(k.name, conf.Task, v)

		var logID string
		logID, err = comm.SendTestLog(ctx, td, &log)
		if err != nil {
			// continue on error to let the other logs be posted
			logger.Task().Errorf("problem posting log: %v", err)
		}
		testResult.LogId = logID
		results = append(results, testResult)
	}
	logger.Task().Info("Finished posting logs to server")

	logger.Task().Info("Sending parsed results to server...")
	if err := comm.SendTestResults(ctx, td, &task.LocalTestResults{
		Results: results,
	}); err != nil {
		logger.Task().Errorf("problem posting parsed results to the server: %+v", err)
		return errors.Wrap(err, "problem sending test results")
	}

	logger.Task().Info("Successfully sent parsed results to server")
	return nil
}

func (c *goTest2JSONCommand) loadJSONFile(file string, logger client.LoggerProducer, conf *model.TaskConfig) ([]*goTest2JSONTestEvent, error) {
	filePath := file
	if !path.IsAbs(filePath) {
		filePath = path.Join(conf.WorkDir, filePath)
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.Task().Errorf("Failed to open '%s'", filePath)
		return nil, errors.Wrapf(err, "failed to open: %s", filePath)
	}

	lines := bytes.Split(data, []byte("\n"))
	out := make([]*goTest2JSONTestEvent, 0, len(lines))
	for i := range lines {
		if len(lines[i]) == 0 {
			continue
		}

		test2JSON, err := parseGoTest2JSON(lines[i])
		if err != nil {
			logger.Task().Errorf("failed to parse %s:%d", filePath, i+1)
			continue
		}

		out = append(out, test2JSON)
	}

	return out, nil
}

func parseGoTest2JSON(bytes []byte) (*goTest2JSONTestEvent, error) {
	t := goTest2JSONTestEvent{}
	if err := json.Unmarshal(bytes, &t); err != nil {
		return nil, errors.Wrap(err, "failed to parse test2json")
	}
	return &t, nil
}

type goTest2JSONMergedTestEvent struct {
	Output    []string
	Status    string
	StartTime time.Time
	EndTime   time.Time
}

func processParsedJSONFile(data []*goTest2JSONTestEvent) map[goTest2JSONKey]*goTest2JSONMergedTestEvent {
	iteration := map[string]int{}
	m := map[goTest2JSONKey]*goTest2JSONMergedTestEvent{}

	for i := range data {
		key := goTest2JSONKey{
			name:      data[i].Test,
			iteration: iteration[data[i].Test],
		}
		if len(data[i].Test) == 0 {
			key.name = fmt.Sprintf("package-%s", data[i].Package)
		}
		if _, ok := m[key]; !ok {
			m[key] = &goTest2JSONMergedTestEvent{StartTime: data[i].Time}
		}

		switch data[i].Action {
		case "run":
			m[key].StartTime = data[i].Time

		case "pass", "fail", "skip":
			m[key].Status = data[i].Action
			m[key].EndTime = data[i].Time

			// For the package level results, we need to compute
			// the start time
			if strings.HasPrefix(key.name, "package-") {
				elapsedNano := data[i].Elapsed * float64(time.Second)
				m[key].StartTime = m[key].EndTime.Add(-time.Duration(elapsedNano))
			}
			// Benchmark test results do not provide accurate timing,
			// so we just zero it
			if strings.HasPrefix(key.name, "Benchmark") {
				m[key].StartTime = data[i].Time
			}

			iteration[data[i].Test] += 1

		case "bench":
			// benchmarks do not give you an average iteration
			// time, so we can't provide an accurate Start and end time
			m[key].Status = "pass"
			m[key].StartTime = data[i].Time
			m[key].EndTime = data[i].Time

		case "pause", "cont", "output":
			m[key].Output = append(m[key].Output, strings.TrimRightFunc(data[i].Output, unicode.IsSpace))
			// test2json does not guarantee that all tests will
			// have a "pass" or "fail" event (ex: panics), so
			// this allows those tests to have an end time
			if len(m[key].Status) == 0 {
				m[key].EndTime = data[i].Time
			}
		}
	}

	return m
}

func goMergedTest2JSONToTestResult(key string, t *task.Task, test2JSON *goTest2JSONMergedTestEvent) (task.TestResult, model.TestLog) {
	result := task.TestResult{
		TestFile:  key,
		LineNum:   1,
		Status:    evergreen.TestFailedStatus,
		StartTime: float64(test2JSON.StartTime.Unix()),
		EndTime:   float64(test2JSON.EndTime.Unix()),
	}
	switch test2JSON.Status {
	case "pass":
		result.Status = evergreen.TestSucceededStatus
	case "skip":
		result.Status = evergreen.TestSkippedStatus
	}

	log := model.TestLog{
		Name:          key,
		Task:          t.Id,
		TaskExecution: t.Execution,
		Lines:         test2JSON.Output,
	}

	return result, log
}
