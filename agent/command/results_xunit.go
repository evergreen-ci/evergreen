package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
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

func xunitResultsFactory() Command   { return &xunitResults{} }
func (c *xunitResults) Name() string { return "attach.xunit_results" }

// ParseParams reads and validates the command parameters. This is required
// to satisfy the 'Command' interface
func (c *xunitResults) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding '%s' params", c.Name())
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
		catcher.Add(err)
	}

	return errors.Wrapf(catcher.Resolve(), "problem expanding paths")
}

// Execute carries out the AttachResultsCommand command - this is required
// to satisfy the 'Command' interface
func (c *xunitResults) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	if err := c.expandParams(conf); err != nil {
		return err
	}

	errChan := make(chan error)
	go func() {
		errChan <- c.parseAndUploadResults(ctx, conf, logger, comm)
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of attach xunit results command")
		return nil
	}
}

// getFilePaths is a helper function that returns a slice of all absolute paths
// which match the given file path parameters.
func getFilePaths(workDir string, files []string) ([]string, error) {
	catcher := grip.NewBasicCatcher()
	out := []string{}

	for _, fileSpec := range files {
		paths, err := filepath.Glob(filepath.Join(workDir, fileSpec))
		catcher.Add(err)
		out = append(out, paths...)
	}

	if catcher.HasErrors() {
		return nil, errors.Wrapf(catcher.Resolve(), "%d incorrect file specifications", catcher.Len())
	}

	return out, nil
}

func (c *xunitResults) parseAndUploadResults(ctx context.Context, conf *internal.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator) error {

	cumulative := testcaseAccumulator{
		tests:           []task.TestResult{},
		logs:            []*model.TestLog{},
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
	for _, reportFileLoc := range reportFilePaths {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		stat, err := os.Stat(reportFileLoc)
		if os.IsNotExist(err) {
			logger.Task().Infof("result file '%s' does not exist", reportFileLoc)
			continue
		}

		if stat.IsDir() {
			logger.Task().Infof("result file '%s' is a directory", reportFileLoc)
			continue
		}

		file, err = os.Open(reportFileLoc)
		if err != nil {
			return errors.Wrap(err, "couldn't open xunit file")
		}

		testSuites, err = parseXMLResults(file)
		if err != nil {
			catcher := grip.NewBasicCatcher()
			catcher.Wrap(err, "error parsing xunit file")
			catcher.Wrap(file.Close(), "closing xunit file")
			return catcher.Resolve()
		}

		if err = file.Close(); err != nil {
			return errors.Wrap(err, "error closing xunit file")
		}

		// go through all the tests
		for idx, suite := range testSuites {
			cumulative = addTestCasesForSuite(suite, idx, conf, cumulative)
		}
	}

	if len(cumulative.tests) == 0 {
		return errors.New("no test results found")
	}

	succeeded := 0
	for i, log := range cumulative.logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		logId, err := sendTestLog(ctx, comm, conf, log)
		if err != nil {
			logger.Task().Warningf("problem uploading logs for %s", log.Name)
			continue
		} else {
			succeeded++
		}
		cumulative.tests[cumulative.logIdxToTestIdx[i]].LogId = logId
		cumulative.tests[cumulative.logIdxToTestIdx[i]].LineNum = 1
	}
	logger.Task().Infof("Attach test logs succeeded for %d of %d files", succeeded, len(cumulative.logs))

	return sendTestResults(ctx, comm, logger, conf, &task.LocalTestResults{Results: cumulative.tests})
}

type testcaseAccumulator struct {
	tests           []task.TestResult
	logs            []*model.TestLog
	logIdxToTestIdx []int
}

func addTestCasesForSuite(suite testSuite, idx int, conf *internal.TaskConfig, cumulative testcaseAccumulator) testcaseAccumulator {
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
		test, log := tc.toModelTestResultAndLog(conf)
		if log != nil {
			if suite.SysOut != "" {
				log.Lines = append(log.Lines, "system-out:", suite.SysOut)
			}
			if suite.SysErr != "" {
				log.Lines = append(log.Lines, "system-err:", suite.SysErr)
			}
			cumulative.logs = append(cumulative.logs, log)
			cumulative.logIdxToTestIdx = append(cumulative.logIdxToTestIdx, len(cumulative.tests))
		}
		cumulative.tests = append(cumulative.tests, test)
	}
	if suite.NestedSuites != nil {
		cumulative = addTestCasesForSuite(*suite.NestedSuites, idx, conf, cumulative)
	}
	return cumulative
}
