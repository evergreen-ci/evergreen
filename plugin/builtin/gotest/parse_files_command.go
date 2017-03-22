package gotest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
)

// ParseFilesCommand is a struct implementing plugin.Command. It is used to parse a file or
// series of files containing the output of go tests, and send the results back to the server.
type ParseFilesCommand struct {
	// a list of filename blobs to include
	// e.g. "monitor.suite", "output/*"
	Files []string `mapstructure:"files" plugin:"expand"`
}

// Name returns the string name for the parse files command.
func (pfCmd *ParseFilesCommand) Name() string {
	return ParseFilesCommandName
}

func (pfCmd *ParseFilesCommand) Plugin() string {
	return GotestPluginName
}

// ParseParams reads the specified map of parameters into the ParseFilesCommand struct, and
// validates that at least one file pattern is specified.
func (pfCmd *ParseFilesCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, pfCmd); err != nil {
		return fmt.Errorf("error decoding '%v' params: %v", pfCmd.Name(), err)
	}

	if len(pfCmd.Files) == 0 {
		return fmt.Errorf("error validating '%v' params:" +
			" must specify at least one file pattern to parse")
	}
	return nil
}

// Execute parses the specified output files and sends the test results found in them
// back to the server.
func (pfCmd *ParseFilesCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, taskConfig *model.TaskConfig,
	stop chan bool) error {

	if err := plugin.ExpandValues(pfCmd, taskConfig.Expansions); err != nil {
		msg := fmt.Sprintf("error expanding params: %v", err)
		pluginLogger.LogTask(slogger.ERROR, "Error parsing gotest files: %v", msg)
		return fmt.Errorf(msg)
	}

	// make sure the file patterns are relative to the task's working directory
	for idx, file := range pfCmd.Files {
		pfCmd.Files[idx] = filepath.Join(taskConfig.WorkDir, file)
	}

	// will be all files containing test results
	outputFiles, err := pfCmd.AllOutputFiles()
	if err != nil {
		return fmt.Errorf("error obtaining names of output files: %v", err)
	}

	// make sure we're parsing something
	if len(outputFiles) == 0 {
		return fmt.Errorf("no files found to be parsed")
	}

	// parse all of the files
	logs, results, err := ParseTestOutputFiles(outputFiles, stop, pluginLogger, taskConfig)
	if err != nil {
		return fmt.Errorf("error parsing output results: %v", err)
	}

	// ship all of the test logs off to the server
	pluginLogger.LogTask(slogger.INFO, "Sending test logs to server...")
	allResults := []*TestResult{}
	for idx, log := range logs {

		logId := ""
		if logId, err = pluginCom.TaskPostTestLog(&log); err != nil {
			// continue on error to let the other logs be posted
			pluginLogger.LogTask(slogger.ERROR, "Error posting log: %v", err)
		}

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId
			allResults = append(allResults, result)
		}

	}
	pluginLogger.LogTask(slogger.INFO, "Finished posting logs to server")

	// convert everything
	resultsAsModel := ToModelTestResults(taskConfig.Task, allResults)

	// ship the parsed results off to the server
	pluginLogger.LogTask(slogger.INFO, "Sending parsed results to server...")
	if err := pluginCom.TaskPostResults(&resultsAsModel); err != nil {
		return fmt.Errorf("error posting parsed results to server: %v", err)
	}
	pluginLogger.LogTask(slogger.INFO, "Successfully sent parsed results to server")

	return nil

}

// AllOutputFiles creates a list of all test output files that will be parsed, by expanding
// all of the file patterns specified to the command.
func (pfCmd *ParseFilesCommand) AllOutputFiles() ([]string, error) {

	outputFiles := []string{}

	// walk through all specified file patterns
	for _, pattern := range pfCmd.Files {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("error expanding file patterns: %v", err)
		}
		outputFiles = append(outputFiles, matches...)
	}

	// uniquify the list
	asSet := map[string]bool{}
	for _, file := range outputFiles {
		asSet[file] = true
	}
	outputFiles = []string{}
	for file := range asSet {
		outputFiles = append(outputFiles, file)
	}

	return outputFiles, nil

}

// ParseTestOutputFiles parses all of the files that are passed in, and returns the
// test logs and test results found within.
func ParseTestOutputFiles(outputFiles []string, stop chan bool,
	pluginLogger plugin.Logger, taskConfig *model.TaskConfig) ([]model.TestLog,
	[][]*TestResult, error) {

	var results [][]*TestResult
	var logs []model.TestLog

	// now, open all the files, and parse the test results
	for _, outputFile := range outputFiles {
		// kill the execution if API server requests
		select {
		case <-stop:
			return nil, nil, fmt.Errorf("command was stopped")
		default:
			// no stop signal
		}

		// assume that the name of the file, stripping off the ".suite" extension if present,
		// is the name of the suite being tested
		_, suiteName := filepath.Split(outputFile)
		suiteName = strings.TrimSuffix(suiteName, ".suite")

		// open the file
		fileReader, err := os.Open(outputFile)
		if err != nil {
			// don't bomb out on a single bad file
			pluginLogger.LogTask(slogger.ERROR, "Unable to open file '%v' for parsing: %v",
				outputFile, err)
			continue
		}
		defer fileReader.Close()

		// parse the output logs
		parser := &VanillaParser{Suite: suiteName}
		if err := parser.Parse(fileReader); err != nil {
			// continue on error
			pluginLogger.LogTask(slogger.ERROR, "Error parsing file '%v': %v",
				outputFile, err)
			continue
		}

		// build up the test logs
		logLines := parser.Logs()
		testLog := model.TestLog{
			Name:          suiteName,
			Task:          taskConfig.Task.Id,
			TaskExecution: taskConfig.Task.Execution,
			Lines:         logLines,
		}
		// save the results
		results = append(results, parser.Results())
		logs = append(logs, testLog)

	}
	return logs, results, nil
}

// ToModelTestResults converts the implementation of TestResults native
// to the gotest plugin to the implementation used by MCI tasks
func ToModelTestResults(t *task.Task, results []*TestResult) task.TestResults {
	var modelResults []task.TestResult
	for _, res := range results {
		// start and end are times that we don't know,
		// represented as a 64bit floating point (epoch time fraction)
		var start float64 = float64(time.Now().Unix())
		var end float64 = start + res.RunTime.Seconds()
		var status string
		switch res.Status {
		// as long as we use a regex, it should be impossible to
		// get an incorrect status code
		case PASS:
			status = evergreen.TestSucceededStatus
		case SKIP:
			status = evergreen.TestSkippedStatus
		case FAIL:
			status = evergreen.TestFailedStatus
		}
		convertedResult := task.TestResult{
			TestFile:  res.Name,
			Status:    status,
			StartTime: start,
			EndTime:   end,
			LineNum:   res.StartLine - 1,
			LogId:     res.LogId,
		}
		modelResults = append(modelResults, convertedResult)
	}
	return task.TestResults{modelResults}
}
