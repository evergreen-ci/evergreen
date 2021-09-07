package command

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// goTestResults is a struct implementing plugin.Command. It is used to parse a file or
// series of files containing the output of go tests, and send the results back to the server.
type goTestResults struct {
	// a list of filename blobs to include
	// e.g. "monitor.suite", "output/*"
	Files []string `mapstructure:"files" plugin:"expand"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// no files are found to be parsed.
	OptionalOutput   string `mapstructure:"optional_output" plugin:"expand"`
	outputIsOptional bool

	base
}

func goTestFactory() Command          { return &goTestResults{} }
func (c *goTestResults) Name() string { return "gotest.parse_files" }

// ParseParams reads the specified map of parameters into the goTestResults struct, and
// validates that at least one file pattern is specified.
func (c *goTestResults) ParseParams(params map[string]interface{}) error {
	var err error
	if err = mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
	}

	if c.OptionalOutput != "" {
		c.outputIsOptional, err = strconv.ParseBool(c.OptionalOutput)
		if err != nil {
			return errors.WithStack(err)
		}
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
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		err = errors.Wrap(err, "error expanding params")
		logger.Task().Errorf("Error parsing goTest files: %+v", err)
		return err
	}

	// All file patterns should be relative to the task's working directory.
	for i, file := range c.Files {
		c.Files[i] = filepath.Join(conf.WorkDir, file)
	}

	// will be all files containing test results
	outputFiles, err := globFiles(c.Files...)
	if err != nil {
		return errors.Wrap(err, "obtaining names of output files")
	}

	// make sure that we're parsing something or have optional parameter
	if len(outputFiles) == 0 {
		if c.outputIsOptional {
			return nil
		} else {
			return errors.New("no files found to be parsed")
		}
	}

	// parse all of the files
	logs, results, err := parseTestOutputFiles(ctx, logger, conf, outputFiles)
	if err != nil {
		return errors.Wrap(err, "error parsing output results")
	}

	if err := sendTestLogsAndResults(ctx, comm, logger, conf, logs, results); err != nil {
		return errors.Wrap(err, "sending test logs and test results")
	}

	return nil
}

// globFiles returns a unique set of files that match the given glob patterns.
func globFiles(patterns ...string) ([]string, error) {
	var matchedFiles []string

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, errors.Wrapf(err, "globbing file pattern '%s'", pattern)
		}
		matchedFiles = append(matchedFiles, matches...)
	}

	// Uniquify the matches
	asSet := map[string]bool{}
	for _, match := range matchedFiles {
		asSet[match] = true
	}
	matchedFiles = []string{}
	for match := range asSet {
		matchedFiles = append(matchedFiles, match)
	}

	return matchedFiles, nil
}

// parseTestOutput parses the test results and logs from a single output source.
func parseTestOutput(ctx context.Context, conf *internal.TaskConfig, report io.Reader, suiteName string) (model.TestLog, []task.TestResult, error) {
	// parse the output logs
	parser := &goTestParser{}
	if err := parser.Parse(report); err != nil {
		return model.TestLog{}, nil, errors.Wrap(err, "parsing file")
	}

	if len(parser.order) == 0 && len(parser.logs) == 0 {
		return model.TestLog{}, nil, errors.New("no results found")
	}

	// build up the test logs
	logLines := parser.Logs()
	testLog := model.TestLog{
		Name:          suiteName,
		Task:          conf.Task.Id,
		TaskExecution: conf.Task.Execution,
		Lines:         logLines,
	}

	return testLog, ToModelTestResults(parser.Results(), suiteName).Results, nil
}

// parseTestOutputFiles parses all of the files that are passed in, and returns
// the test logs and test results found within.
func parseTestOutputFiles(ctx context.Context, logger client.LoggerProducer,
	conf *internal.TaskConfig, outputFiles []string) ([]model.TestLog, [][]task.TestResult, error) {

	var results [][]task.TestResult
	var logs []model.TestLog

	// now, open all the files, and parse the test results
	for _, outputFile := range outputFiles {
		if ctx.Err() != nil {
			return nil, nil, errors.New("command was stopped")
		}

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
		defer fileReader.Close() //nolint: evg-lint

		log, result, err := parseTestOutput(ctx, conf, fileReader, suiteName)
		if err != nil {
			// continue on error
			logger.Task().Errorf("Error parsing file '%s': %v", outputFile, err)
			continue
		}

		// save the results
		results = append(results, result)
		logs = append(logs, log)
	}

	if len(results) == 0 && len(logs) == 0 {
		return nil, nil, errors.New("go test output files contained no results")
	}

	return logs, results, nil
}
