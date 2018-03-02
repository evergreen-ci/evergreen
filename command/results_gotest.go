package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// goTestResults is a struct implementing plugin.Command. It is used to parse a file or
// series of files containing the output of go tests, and send the results back to the server.
type goTestResults struct {
	// a list of filename blobs to include
	// e.g. "monitor.suite", "output/*"
	Files []string `mapstructure:"files" plugin:"expand"`
	base
}

func goTestFactory() Command          { return &goTestResults{} }
func (c *goTestResults) Name() string { return "gotest.parse_files" }

// ParseParams reads the specified map of parameters into the goTestResults struct, and
// validates that at least one file pattern is specified.
func (c *goTestResults) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if len(c.Files) == 0 {
		return errors.Errorf("error validating params: must specify at least one "+
			"file pattern to parse: '%+v'", params)
	}
	return nil
}

// Execute parses the specified output files and sends the test results found in them
// back to the server.
func (c *goTestResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		err = errors.Wrap(err, "error expanding params")
		logger.Task().Errorf("Error parsing goTest files: %+v", err)
		return err
	}

	// make sure the file patterns are relative to the task's working directory
	for idx, file := range c.Files {
		c.Files[idx] = filepath.Join(conf.WorkDir, file)
	}

	// will be all files containing test results
	outputFiles, err := c.allOutputFiles()
	if err != nil {
		return errors.Wrap(err, "error obtaining names of output files")
	}

	// make sure we're parsing something
	if len(outputFiles) == 0 {
		return errors.New("no files found to be parsed")
	}

	// parse all of the files
	logs, results, err := parseTestOutputFiles(ctx, logger, conf, outputFiles)
	if err != nil {
		return errors.Wrap(err, "error parsing output results")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	// ship all of the test logs off to the server
	logger.Task().Info("Sending test logs to server...")
	allResults := task.LocalTestResults{}
	for idx, log := range logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		var logId string

		logId, err = comm.SendTestLog(ctx, td, &log)
		if err != nil {
			// continue on error to let the other logs be posted
			logger.Task().Errorf("problem posting log: %v", err)
		}

		// add all of the test results that correspond to that log to the
		// full list of results
		for _, result := range results[idx] {
			result.LogId = logId

			allResults.Results = append(allResults.Results, result)
		}

	}
	logger.Task().Info("Finished posting logs to server")

	// ship the parsed results off to the server
	logger.Task().Info("Sending parsed results to server...")

	if err := comm.SendTestResults(ctx, td, &allResults); err != nil {
		logger.Task().Errorf("problem posting parsed results to the server: %+v", err)
		return errors.Wrap(err, "problem sending test results")
	}

	logger.Task().Info("Successfully sent parsed results to server")

	return nil
}

// AllOutputFiles creates a list of all test output files that will be parsed, by expanding
// all of the file patterns specified to the command.
func (c *goTestResults) allOutputFiles() ([]string, error) {

	outputFiles := []string{}

	// walk through all specified file patterns
	for _, pattern := range c.Files {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, errors.Wrap(err, "error expanding file patterns")
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
func parseTestOutputFiles(ctx context.Context, logger client.LoggerProducer,
	conf *model.TaskConfig, outputFiles []string) ([]model.TestLog, [][]task.TestResult, error) {

	var results [][]task.TestResult
	var logs []model.TestLog

	catcher := grip.NewSimpleCatcher()
	// now, open all the files, and parse the test results
	for _, outputFile := range outputFiles {
		if ctx.Err() != nil {
			return nil, nil, errors.New("command was stopped")
		}

		// assume that the name of the file, stripping off the ".suite" extension if present,
		// is the name of the suite being tested
		_, suiteName := filepath.Split(outputFile)
		suiteName = strings.TrimSuffix(suiteName, ".suite")

		// open the file
		fileReader, err := os.Open(outputFile)
		if err != nil {
			// don't bomb out on a single bad file
			logger.Task().Errorf("Unable to open file '%s' for parsing: %v",
				outputFile, err)
			continue
		}

		// parse the output logs
		parser := &goTestParser{Suite: suiteName}
		if err := parser.Parse(fileReader); err != nil {
			catcher.Add(fileReader.Close())
			// continue on error
			logger.Task().Errorf("Error parsing file '%s': %v", outputFile, err)
			continue
		}

		// build up the test logs
		logLines := parser.Logs()
		testLog := model.TestLog{
			Name:          suiteName,
			Task:          conf.Task.Id,
			TaskExecution: conf.Task.Execution,
			Lines:         logLines,
		}
		// save the results
		results = append(results, ToModelTestResults(parser.Results()).Results)
		logs = append(logs, testLog)
		catcher.Add(fileReader.Close())
	}
	return logs, results, catcher.Resolve()
}
