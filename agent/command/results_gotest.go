package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	goTestMalformedOutputNumAttribute = "evergreen.command.gotest.parse_files.malformed_output_num"
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
func (c *goTestResults) ParseParams(params map[string]any) error {
	var err error
	if err = mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.OptionalOutput != "" {
		c.outputIsOptional, err = strconv.ParseBool(c.OptionalOutput)
		if err != nil {
			return errors.Wrap(err, "parsing optional output parameter as a boolean")
		}
	}

	if len(c.Files) == 0 {
		return errors.Errorf("must specify at least one file pattern to parse")
	}
	return nil
}

// Execute parses the specified output files and sends the test results found in them
// back to the server.
func (c *goTestResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// All file patterns should be relative to the task's working directory.
	SetWorkdirBoundaryAttribute(ctx, conf, c.Files...)
	for i, file := range c.Files {
		c.Files[i] = GetWorkingDirectory(conf, file)
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
		}

		return errors.New("no files found to be parsed")
	}

	// parse all of the files
	logs, results, malformed, err := parseTestOutputFiles(ctx, logger, conf, outputFiles)
	if err != nil {
		return errors.Wrap(err, "parsing output results")
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int(goTestMalformedOutputNumAttribute, len(malformed)))

	if !conf.Task.MustHaveResults && len(results) == 0 {
		return nil
	}

	if err := sendTestLogsAndResults(ctx, comm, logger, conf, logs, results); err != nil {
		return errors.Wrap(err, "sending test logs and test results")
	}

	// Malformed output is reported after the results are sent so that the results, which are still
	// usable, are available even though this command fails.
	if len(malformed) > 0 {
		return errors.Errorf("test output is malformed, which usually means output from the program under test interleaved with go test's own output: '%s'. Consider sending the program's own logging to a separate file or to stderr", strings.Join(malformed, "; "))
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

// parseTestOutput parses the test results and logs from a single output source. It also returns any
// malformed test end lines it encountered.
func parseTestOutput(ctx context.Context, conf *internal.TaskConfig, report io.Reader, suiteName string) (testlog.TestLog, []testresult.TestResult, []malformedLine, error) {
	parser := &goTestParser{}
	if err := parser.Parse(report); err != nil {
		return testlog.TestLog{}, nil, nil, errors.Wrap(err, "parsing file")
	}

	if len(parser.order) == 0 && len(parser.logs) == 0 {
		return testlog.TestLog{}, nil, nil, errors.New("no results found")
	}

	logLines := parser.Logs()
	logs := testlog.TestLog{
		Name:          suiteName,
		Task:          conf.Task.Id,
		TaskExecution: conf.Task.Execution,
		Lines:         logLines,
	}

	return logs, ToModelTestResults(parser.Results(), suiteName), parser.malformedLines, nil
}

// parseTestOutputFiles parses all of the files that are passed in, and returns the test logs and
// test results found within, plus a description of each malformed test end line encountered.
func parseTestOutputFiles(ctx context.Context, logger client.LoggerProducer, conf *internal.TaskConfig, outputFiles []string) ([]testlog.TestLog, []testresult.TestResult, []string, error) {
	var (
		allResults []testresult.TestResult
		logs       []testlog.TestLog
		malformed  []string
	)

	for _, outputFile := range outputFiles {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, errors.Wrap(err, "canceled while processing test output files")
		}

		_, suiteName := filepath.Split(outputFile)
		suiteName = strings.TrimSuffix(suiteName, ".suite")

		fileReader, err := os.Open(outputFile)
		if err != nil {
			logger.Task().Error(ctx, errors.Wrapf(err, "opening file '%s' for parsing", outputFile))
			continue
		}
		defer fileReader.Close() //nolint: evg-lint

		log, results, malformedLines, err := parseTestOutput(ctx, conf, fileReader, suiteName)
		if err != nil {
			logger.Task().Error(ctx, errors.Wrapf(err, "parsing file '%s'", outputFile))
			continue
		}
		if len(malformedLines) > 0 {
			descriptions := make([]string, 0, len(malformedLines))
			for _, l := range malformedLines {
				description := fmt.Sprintf("line %d recovered status '%s' for test '%s'", l.lineNum, l.status, l.testName)
				descriptions = append(descriptions, description)
				malformed = append(malformed, fmt.Sprintf("file '%s' %s", outputFile, description))
			}
			logger.Task().Errorf(ctx, "Test output file '%s' has malformed test end lines: '%s'. The tests' statuses were recovered, but their runtimes may be missing.", outputFile, strings.Join(descriptions, ", "))
		}

		allResults = append(allResults, results...)
		logs = append(logs, log)
	}

	return logs, allResults, malformed, nil
}
