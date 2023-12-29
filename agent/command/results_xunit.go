package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// xunitResults reads in an xml file of xunit
// type results and converts them to a format MCI can use
type xunitResults struct {
	// File describes the relative path of the file to be sent. Supports globbing.
	// Note that this can also be described via expansions.
	File  string   `mapstructure:"file" plugin:"expand"`
	Files []string `mapstructure:"files" plugin:"expand"`
	base
}

const (
	systemOut = "system-out:"
	systemErr = "system-err:"
)

func xunitResultsFactory() Command   { return &xunitResults{} }
func (c *xunitResults) Name() string { return evergreen.AttachXUnitResultsCommandName }

// ParseParams reads and validates the command parameters. This is required
// to satisfy the 'Command' interface
func (c *xunitResults) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.File == "" && len(c.Files) == 0 {
		return errors.New("must specify at least one file")
	}

	return nil
}

// Expand the parameter appropriately
func (c *xunitResults) expandParams(conf *internal.TaskConfig) error {
	if c.File != "" {
		c.Files = append(c.Files, c.File)
	}

	catcher := grip.NewBasicCatcher()

	var err error
	for idx, f := range c.Files {
		c.Files[idx], err = conf.Expansions.ExpandString(f)
		catcher.Wrapf(err, "expanding file '%s'", f)
	}

	return catcher.Resolve()
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (c *xunitResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	errChan := make(chan error)
	go func() {
		err := c.parseAndUploadResults(ctx, conf, logger, comm)
		select {
		case errChan <- err:
			return
		case <-ctx.Done():
			logger.Task().Infof("Context canceled waiting to parse and upload results: %s.", ctx.Err())
			return
		}
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Infof("Canceled while parsing and uploading results for command '%s': %s.", c.Name(), ctx.Err())
		return nil
	}
}

// getFilePaths is a helper function that returns a slice of all absolute paths
// which match the given file path parameters.
func getFilePaths(workDir string, files []string) ([]string, error) {
	catcher := grip.NewBasicCatcher()
	out := []string{}

	for _, fileSpec := range files {
		relativeToWorkDir := strings.TrimPrefix(filepath.ToSlash(fileSpec), filepath.ToSlash(workDir))
		path := filepath.Join(workDir, relativeToWorkDir)
		paths, err := filepath.Glob(path)
		catcher.Add(err)
		out = append(out, paths...)
	}

	if catcher.HasErrors() {
		return nil, errors.Wrapf(catcher.Resolve(), "%d incorrect file specifications", catcher.Len())
	}
	// Only error for no files if the user provided files.
	if len(out) == 0 && len(files) > 0 {
		return nil, errors.New("Files parameter was provided but no XML files matched")
	}

	return out, nil
}

func (c *xunitResults) parseAndUploadResults(ctx context.Context, conf *internal.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator) error {

	cumulative := testcaseAccumulator{
		tests:           []testresult.TestResult{},
		logs:            []*testlog.TestLog{},
		logIdxToTestIdx: []int{},
	}

	reportFilePaths, err := getFilePaths(conf.WorkDir, c.Files)
	if err != nil {
		return errors.WithStack(err)
	}

	var (
		file       *os.File
		testSuites []testSuite
	)
	numInvalid := 0
	for _, reportFileLoc := range reportFilePaths {
		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "canceled while parsing xunit file '%s'", reportFileLoc)
		}

		stat, err := os.Stat(reportFileLoc)
		if os.IsNotExist(err) {
			numInvalid += 1
			logger.Task().Infof("Result file '%s' does not exist.", reportFileLoc)
			continue
		}

		if stat.IsDir() {
			numInvalid += 1
			logger.Task().Infof("Result file '%s' is a directory, not a file.", reportFileLoc)
			continue
		}

		file, err = os.Open(reportFileLoc)
		if err != nil {
			return errors.Wrapf(err, "opening xunit file '%s'", reportFileLoc)
		}

		testSuites, err = parseXMLResults(file)
		if err != nil {
			catcher := grip.NewBasicCatcher()
			catcher.Wrapf(err, "parsing xunit file '%s'", reportFileLoc)
			catcher.Wrapf(file.Close(), "closing xunit file '%s'", reportFileLoc)
			return catcher.Resolve()
		}

		if err = file.Close(); err != nil {
			return errors.Wrapf(err, "closing xunit file '%s'", reportFileLoc)
		}

		// go through all the tests
		for idx, suite := range testSuites {
			cumulative = addTestCasesForSuite(suite, idx, conf, cumulative, logger)
		}
	}
	if len(reportFilePaths) == numInvalid {
		return errors.New("all given file paths do not exist or are directories")
	}

	succeeded := 0
	for i, log := range cumulative.logs {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while sending test logs")
		}

		if err := sendTestLog(ctx, comm, conf, log); err != nil {
			logger.Task().Error(errors.Wrap(err, "sending test log"))
			continue
		} else {
			succeeded++
		}
		cumulative.tests[cumulative.logIdxToTestIdx[i]].LineNum = 1
	}
	logger.Task().Infof("Posting test logs succeeded for %d of %d logs.", succeeded, len(cumulative.logs))
	if len(cumulative.tests) > 0 {
		return sendTestResults(ctx, comm, logger, conf, cumulative.tests)
	}
	return nil
}

type testcaseAccumulator struct {
	tests           []testresult.TestResult
	logs            []*testlog.TestLog
	logIdxToTestIdx []int
}

func addTestCasesForSuite(suite testSuite, idx int, conf *internal.TaskConfig, cumulative testcaseAccumulator, logger client.LoggerProducer) testcaseAccumulator {
	if len(suite.TestCases) == 0 && suite.Error != nil {
		// if no test cases but an error, generate a default test case
		tc := testCase{
			Name:  suite.Name,
			Time:  suite.Time,
			Error: suite.Error,
		}
		if tc.Name == "" {
			tc.Name = fmt.Sprintf("Unnamed Test-%d", idx)
		}
		suite.TestCases = append(suite.TestCases, tc)
	}
	for _, tc := range suite.TestCases {
		// logs are only created when a test case does not succeed
		test, log := tc.toModelTestResultAndLog(conf, logger)
		if log != nil {
			if systemLogs := constructSystemLogs(suite.SysOut, suite.SysErr); len(systemLogs) > 0 {
				log.Lines = append(log.Lines, systemLogs...)
			}
			cumulative.logs = append(cumulative.logs, log)
			cumulative.logIdxToTestIdx = append(cumulative.logIdxToTestIdx, len(cumulative.tests))
		}
		cumulative.tests = append(cumulative.tests, test)
	}
	if suite.NestedSuites != nil {
		cumulative = addTestCasesForSuite(*suite.NestedSuites, idx, conf, cumulative, logger)
	}
	return cumulative
}

func constructSystemLogs(sysOut, sysErr string) []string {
	var lines []string
	if sysOut != "" {
		lines = append(lines, systemOut, sysOut)
	}
	if sysErr != "" {
		lines = append(lines, systemErr, sysErr)
	}
	return lines
}
