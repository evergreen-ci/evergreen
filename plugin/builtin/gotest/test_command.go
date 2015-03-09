package gotest

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type RunTestCommand struct {
	WorkDir string       `mapstructure:"working_dir" plugin:"expand"`
	Tests   []TestConfig `mapstructure:"tests" plugin:"expand"`
}

type TestConfig struct {
	Dir                  string   `mapstructure:"dir" plugin:"expand"`
	Args                 string   `mapstructure:"args" plugin:"expand"`
	EnvironmentVariables []string `mapstructure:"environment_variables" plugin:"expand"`
}

func (self *RunTestCommand) Name() string {
	return RunTestCommandName
}

func (self *RunTestCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", self.Name(), err)
	}

	if len(self.Tests) == 0 {
		return fmt.Errorf("error validating '%v' params: need at least one "+
			"test to run", self.Name())
	}
	return nil
}

// getSuiteNameFromDir returns a suitable suite name for a test
// given its directory and position in the test list
func getSuiteNameFromDir(testNumber int, dir string) string {
	base := path.Base(dir)
	return fmt.Sprintf("%v_%v", testNumber, base)
}

func (self *RunTestCommand) Execute(pluginLogger plugin.PluginLogger,
	pluginCom plugin.PluginCommunicator, taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := plugin.ExpandValues(self, taskConfig.Expansions); err != nil {
		msg := fmt.Sprintf("error expanding params: %v", err)
		pluginLogger.LogTask(slogger.ERROR, "Error updating test configs: %v",
			msg)
		return fmt.Errorf(msg)
	}

	// define proper working directory
	if self.WorkDir != "" {
		self.WorkDir = filepath.Join(taskConfig.WorkDir, self.WorkDir)
	} else {
		self.WorkDir = taskConfig.WorkDir
	}
	pluginLogger.LogTask(slogger.INFO,
		"Running tests with working dir '%v'", self.WorkDir)

	if os.Getenv("GOPATH") == "" {
		pluginLogger.LogTask(slogger.WARN, "No GOPATH; setting GOPATH to working dir")
		err := os.Setenv("GOPATH", self.WorkDir)
		if err != nil {
			return err
		}
	}

	var results []TestResult
	allPassed := true

	// run all tests, concat results. Hold onto failures until the end
	for idx, test := range self.Tests {

		// kill the execution if motu wants us to shut down
		select {
		case <-stop:
			return fmt.Errorf("command was stopped")
		default:
			// no stop signal
		}

		// update test directory
		test.Dir = filepath.Join(self.WorkDir, test.Dir)
		suiteName := getSuiteNameFromDir(idx, test.Dir)

		parser := &VanillaParser{Suite: suiteName}
		pluginLogger.LogTask(
			slogger.INFO, "Running go test with '%v' in '%v'", test.Args, test.Dir)
		if len(test.EnvironmentVariables) > 0 {
			pluginLogger.LogTask(
				slogger.INFO, "Adding environment variables to gotest: %#v",
				test.EnvironmentVariables)
		}
		passed, err := RunAndParseTests(test, parser, pluginLogger, stop)

		logLines := parser.Logs()
		for _, log := range logLines {
			pluginLogger.LogTask(slogger.INFO, ">>> %v", log)
		}

		pluginLogger.LogTask(slogger.INFO,
			"Sending logs to API server (%v lines)", len(logLines))
		testLog := &model.TestLog{
			Name:          suiteName,
			Task:          taskConfig.Task.Id,
			TaskExecution: taskConfig.Task.Execution,
			Lines:         logLines,
		}

		_, err = pluginCom.TaskPostTestLog(testLog)
		if err != nil {
			pluginLogger.LogTask(slogger.ERROR, "error posting test log: %v", err)
		}

		if passed != true {
			allPassed = false
			pluginLogger.LogTask(slogger.WARN, "Test suite failed, continuing...")
		}
		if err != nil {
			pluginLogger.LogTask(slogger.ERROR,
				"Error running and parsing test '%v': %v", test.Dir, err)
			continue
		}

		results = append(results, parser.Results()...)
	}

	pluginLogger.LogTask(slogger.INFO, "Sending go test results to server")
	modelResults := ToModelTestResults(taskConfig.Task, results)
	err := pluginCom.TaskPostResults(&modelResults)
	if err != nil {
		return fmt.Errorf("error posting results: %v", err)
	}

	if allPassed {
		return nil
	} else {
		return fmt.Errorf("test failures")
	}
}

func RunAndParseTests(conf TestConfig, parser Parser,
	pluginLogger plugin.PluginLogger, stop <-chan bool) (bool, error) {

	originalDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("could not get working directory: %v", err)
	}

	// cd to given folder, but reset at the end
	if err = os.Chdir(conf.Dir); err != nil {
		return false, fmt.Errorf(
			"could not change working directory to %v from %v: %v",
			conf.Dir, originalDir, err)
	}
	defer os.Chdir(originalDir)

	cmd := exec.Command("sh", "-c", "go test -v "+conf.Args)
	if len(conf.EnvironmentVariables) > 0 {
		cmd = exec.Command("sh", "-c", strings.Join(conf.EnvironmentVariables, " ")+
			" go test -v "+conf.Args)
	}

	// display stderr but continue in the unlikely case we can't get it
	stderr, err := cmd.StderrPipe()
	if err == nil {
		go io.Copy(pluginLogger.GetTaskLogWriter(slogger.ERROR), stderr)
	} else {
		pluginLogger.LogTask(
			slogger.WARN, "couldn't get stderr from command: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("could not read stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf(
			"could not start test 'go test -v %v': %v", conf.Args, err)
	}
	if err := parser.Parse(stdout); err != nil {
		// log and keep going if we have trouble parsing; corrupt output
		// does not mean that the tests are failing--let the exit code
		// determine that!
		pluginLogger.LogTask(slogger.ERROR, "error parsing test output: %v", err)
	}

	// wait in a goroutine, so we can
	// kill running tests if they time out
	waitErrChan := make(chan error)
	go func() {
		waitErrChan <- cmd.Wait()
	}()

	// wait for execution completion or a stop signal
	select {
	case <-stop:
		cmd.Process.Kill()
		return false, fmt.Errorf("command was stopped")
	case err = <-waitErrChan:
		// if an error is returned, the test either failed or failed to
		// be copied properly. We must distinguish these below
		if err != nil {
			// if we finished but failed, return failure with no error
			if cmd.ProcessState.Exited() && !cmd.ProcessState.Success() {
				return false, nil
			} else {
				return false, fmt.Errorf("error ending test: %v", err)
			}
		}
		return true, nil
	}
}

// ToModelTestResults converts the implementation of TestResults native
// to the gotest plugin to the implementation used by MCI tasks
func ToModelTestResults(task *model.Task, results []TestResult) model.TestResults {
	var modelResults []model.TestResult
	for _, res := range results {
		// start and end are times that we don't know,
		// represented as a 64bit floating point (epoch time fraction)
		// ...TODO not sure why we use this schema...
		var start float64 = float64(time.Now().Unix())
		var end float64 = start + res.RunTime.Seconds()
		var status string
		switch res.Status {
		// as long as we use a regex, it should be impossible to
		// get an incorrect status code
		case PASS:
			status = mci.TestSucceededStatus
		case SKIP:
			status = mci.TestSkippedStatus
		case FAIL:
			status = mci.TestFailedStatus
		}
		url := fmt.Sprintf(
			"/test_log/%v/%v/%v#L%v",
			task.Id,
			task.Execution,
			res.SuiteName,
			res.StartLine-1, //current view indexes off 0 instead of 1
		)
		convertedResult := model.TestResult{
			TestFile:  res.Name,
			Status:    status,
			StartTime: start,
			EndTime:   end,
			URL:       url,
		}
		modelResults = append(modelResults, convertedResult)
	}
	return model.TestResults{modelResults}
}
