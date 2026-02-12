package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
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
func (c *xunitResults) ParseParams(params map[string]any) error {
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

// parseXMLFileResult holds the result of parsing a single XML file.
type parseXMLFileResult struct {
	filePath string
	suites   []testSuite
	invalid  bool
	err      error
}

// parseXMLFile parses a single xunit XML file and returns the test suites.
func parseXMLFile(ctx context.Context, filePath string) parseXMLFileResult {
	// Check if context is already cancelled before starting to parse.
	if err := ctx.Err(); err != nil {
		return parseXMLFileResult{filePath: filePath, err: errors.Wrap(err, "context cancelled before parsing")}
	}

	stat, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return parseXMLFileResult{filePath: filePath, invalid: true}
	}
	if stat.IsDir() {
		return parseXMLFileResult{filePath: filePath, invalid: true}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return parseXMLFileResult{filePath: filePath, err: errors.Wrapf(err, "opening xunit file '%s'", filePath)}
	}

	suites, err := parseXMLResults(ctx, file)
	if err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "parsing xunit file '%s'", filePath)
		catcher.Wrapf(file.Close(), "closing xunit file '%s'", filePath)
		return parseXMLFileResult{filePath: filePath, err: catcher.Resolve()}
	}

	if err = file.Close(); err != nil {
		return parseXMLFileResult{filePath: filePath, err: errors.Wrapf(err, "closing xunit file '%s'", filePath)}
	}

	return parseXMLFileResult{filePath: filePath, suites: suites}
}

func (c *xunitResults) parseAndUploadResults(ctx context.Context, conf *internal.TaskConfig,
	logger client.LoggerProducer, comm client.Communicator) error {

	reportFilePaths, err := getFilePaths(conf.WorkDir, c.Files)
	if err != nil {
		return errors.WithStack(err)
	}

	// Parse XML files in parallel using a worker pool.
	jobs := make(chan string, len(reportFilePaths))
	results := make(chan parseXMLFileResult, len(reportFilePaths))

	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		go func() {
			for filePath := range jobs {
				select {
				case <-ctx.Done():
					return
				case results <- parseXMLFile(ctx, filePath):
				}
			}
		}()
	}

	for _, path := range reportFilePaths {
		select {
		case <-ctx.Done():
			close(jobs)
			return errors.Wrap(ctx.Err(), "canceled while queuing files for parsing")
		case jobs <- path:
		}
	}
	close(jobs)

	// Collect parse results and build cumulative test cases.
	cumulative := testcaseAccumulator{
		tests:           []testresult.TestResult{},
		logs:            []*testlog.TestLog{},
		logIdxToTestIdx: []int{},
	}
	var numInvalid int
	catcher := grip.NewBasicCatcher()
	for i := 0; i < len(reportFilePaths); i++ {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "canceled while collecting parse results")
		case result := <-results:
			if result.err != nil {
				catcher.Add(result.err)
				continue
			}
			if result.invalid {
				numInvalid++
				logger.Task().Infof("Result file '%s' does not exist or is a directory.", result.filePath)
				continue
			}
			for idx, suite := range result.suites {
				cumulative = addTestCasesForSuite(suite, idx, conf, cumulative, logger)
			}
		}
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if len(reportFilePaths) == numInvalid {
		return errors.New("all given file paths do not exist or are directories")
	}

	// Upload test logs in parallel using a worker pool.
	type logWork struct {
		idx int
		log *testlog.TestLog
	}
	work := make(chan logWork, len(cumulative.logs))
	for i, log := range cumulative.logs {
		work <- logWork{idx: i, log: log}
	}
	close(work)

	var succeeded int64
	var wg sync.WaitGroup
	opts := redactor.RedactionOptions{
		Expansions:         conf.NewExpansions,
		Redacted:           conf.Redacted,
		InternalRedactions: conf.InternalRedactions,
	}

	numWorkers := runtime.GOMAXPROCS(0)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				if err := ctx.Err(); err != nil {
					logger.Task().Warning(errors.Wrap(err, "context canceled while sending test logs"))
					return
				}
				if err := taskoutput.AppendTestLog(ctx, &conf.Task, opts, item.log); err != nil {
					logger.Task().Error(errors.Wrap(err, "sending test log"))
					continue
				}
				atomic.AddInt64(&succeeded, 1)
				cumulative.tests[cumulative.logIdxToTestIdx[item.idx]].LineNum = 1

				// Yield to allow other goroutines to run and prevent starvation
				// in intense log uploading workflows.
				runtime.Gosched()
			}
		}()
	}
	wg.Wait()

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
