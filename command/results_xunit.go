package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
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
func (c *xunitResults) expandParams(conf *model.TaskConfig) error {
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
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

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

func (c *xunitResults) parseAndUploadResults(ctx context.Context, conf *model.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator) error {

	tests := []task.TestResult{}
	logs := []*model.TestLog{}
	logIdxToTestIdx := []int{}

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
		defer file.Close() // nolint

		testSuites, err = parseXMLResults(file)
		if err != nil {
			return errors.Wrap(err, "error parsing xunit file")
		}

		if err = file.Close(); err != nil {
			return errors.Wrap(err, "error closing xunit file")
		}

		// go through all the tests
		for idx, suite := range testSuites {
			if len(suite.TestCases) == 0 && suite.Error != nil {
				// if no test cases but an error, generate a default test case
				tc := testCase{
					Name:  suite.Name,
					Time:  suite.Time,
					Error: suite.Error,
				}
				if tc.Name == "" {
					tc.Name = fmt.Sprintf("Unamed Test-%d", idx)
				}
				suite.TestCases = append(suite.TestCases, tc)
			}
			for _, tc := range suite.TestCases {
				// logs are only created when a test case does not succeed
				test, log := tc.toModelTestResultAndLog(conf.Task)
				if log != nil {
					if suite.SysOut != "" {
						log.Lines = append(log.Lines, "system-out:", suite.SysOut)
					}
					if suite.SysErr != "" {
						log.Lines = append(log.Lines, "system-err:", suite.SysErr)
					}
					logs = append(logs, log)
					logIdxToTestIdx = append(logIdxToTestIdx, len(tests))
				}
				tests = append(tests, test)
			}
		}
	}

	if len(tests) == 0 {
		return errors.New("no test results found")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	for i, log := range logs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		logID, err := sendJSONLogs(ctx, logger, comm, td, log)
		if err != nil {
			logger.Task().Warningf("problem uploading logs for %s", log.Name)
			continue
		}
		tests[logIdxToTestIdx[i]].LogId = logID
		tests[logIdxToTestIdx[i]].LineNum = 1
	}

	return sendJSONResults(ctx, conf, logger, comm, &task.LocalTestResults{Results: tests})
}
