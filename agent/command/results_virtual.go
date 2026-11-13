package command

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/internal/taskoutput"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	virtualTestResultTypeNative = "native"
	virtualTestResultTypeGo     = "go"
	virtualTestResultTypeXUnit  = "xunit"
)

// parseOptionsForVirtualTask returns the test result parsing options for the
// virtual task identified by info.
func parseOptionsForVirtualTask(conf *internal.TaskConfig, info testresult.TestResultsInfo) testResultParseOptions {
	return testResultParseOptions{
		workDir:       conf.WorkDir,
		taskID:        info.TaskID,
		taskExecution: info.Execution,
	}
}

// ProcessVirtualTaskTestResultFiles parses the given test result files, uploads
// the test logs and test results to S3 on behalf of the virtual task identified
// by info, and returns the test result metadata.
func ProcessVirtualTaskTestResultFiles(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, info testresult.TestResultsInfo, taskCreatedAt time.Time, input VirtualTaskTestResultsInput) (*apimodels.VirtualTaskTestResults, error) {
	parseOpts := parseOptionsForVirtualTask(conf, info)

	logs, results, err := parseVirtualTaskTestResultFileGroups(ctx, parseOpts, logger, input.Files)
	if err != nil {
		return nil, errors.Wrap(err, "parsing test result files")
	}
	if len(results) == 0 {
		return nil, errors.New("no test results found in files")
	}

	if err = uploadVirtualTaskTestLogs(ctx, conf, logger, info, input.LogDir, logs, results); err != nil {
		return nil, errors.Wrap(err, "uploading test logs")
	}

	return uploadVirtualTaskTestResults(ctx, conf, info, taskCreatedAt, input.GetCreatedAt(), results)
}

// parseVirtualTaskTestResultFileGroups parses all test result file group into test logs and results.
func parseVirtualTaskTestResultFileGroups(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, groups []VirtualTaskTestResultFileGroup) ([]testlog.TestLog, []testresult.TestResult, error) {
	var (
		allLogs    []testlog.TestLog
		allResults []testresult.TestResult
	)
	catcher := grip.NewBasicCatcher()
	for _, group := range groups {
		logs, results, err := parseVirtualTaskTestResultFileGroup(ctx, opts, logger, group)
		if err != nil {
			catcher.Wrapf(err, "parsing '%s' test results", group.Type)
			continue
		}
		allLogs = append(allLogs, logs...)
		allResults = append(allResults, results...)
	}
	if catcher.HasErrors() {
		return nil, nil, catcher.Resolve()
	}
	return allLogs, allResults, nil
}

// parseVirtualTaskTestResultFileGroup parses test results and test logs
// from a group of files of a given test result format.
func parseVirtualTaskTestResultFileGroup(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, fileGroup VirtualTaskTestResultFileGroup) ([]testlog.TestLog, []testresult.TestResult, error) {
	if len(fileGroup.Files) == 0 {
		return nil, nil, errors.New("must specify at least one file")
	}

	switch fileGroup.Type {
	case virtualTestResultTypeNative:
		var (
			logs    []testlog.TestLog
			results []testresult.TestResult
		)
		for _, file := range fileGroup.Files {
			fileLogs, fileResults, err := parseNativeResults(opts, file)
			if err != nil {
				return nil, nil, err
			}
			logs = append(logs, fileLogs...)
			results = append(results, fileResults...)
		}
		return logs, results, nil
	case virtualTestResultTypeGo:
		patterns := make([]string, len(fileGroup.Files))
		for i, file := range fileGroup.Files {
			patterns[i] = resolveWorkingDirectory(opts.workDir, file)
		}
		files, err := globFiles(patterns...)
		if err != nil {
			return nil, nil, err
		}
		if len(files) == 0 {
			return nil, nil, errors.New("no files found to be parsed")
		}
		logs, results, _, err := parseGotestOutputFiles(ctx, logger, opts, files)
		return logs, results, err
	case virtualTestResultTypeXUnit:
		cumulative, err := parseXUnitResults(ctx, opts, logger, fileGroup.Files)
		if err != nil {
			return nil, nil, err
		}
		logs := make([]testlog.TestLog, len(cumulative.logs))
		for i, log := range cumulative.logs {
			logs[i] = *log
		}
		return logs, cumulative.tests, nil
	default:
		return nil, nil, errors.Errorf("unrecognized test result type '%s'", fileGroup.Type)
	}
}

// uploadVirtualTaskTestLogs uploads the given parsed test logs, as well as the
// test log files in logDir referenced by the results, on behalf of the virtual
// task identified by info.
func uploadVirtualTaskTestLogs(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, info testresult.TestResultsInfo, logDir string, logs []testlog.TestLog, results []testresult.TestResult) error {
	logPaths := referencedTestLogPaths(logs, results)
	if len(logPaths) > 0 && logDir == "" {
		logger.Task().Warningf(ctx, "Test results for virtual task '%s' reference %d test log file(s), but no log directory was specified, so they will not be uploaded.", info.TaskID, len(logPaths))
		logPaths = nil
	}
	if len(logs) == 0 && len(logPaths) == 0 {
		return nil
	}
	output := conf.Task.TaskOutputInfo
	if output == nil {
		return errors.New("runner task output info is not set")
	}
	vTask := task.Task{
		Id:        info.TaskID,
		Execution: info.Execution,
		Project:   info.Project,
	}

	opts := redactor.RedactionOptions{
		Expansions:         conf.NewExpansions,
		Redacted:           conf.Redacted,
		InternalRedactions: conf.InternalRedactions,
	}

	if len(logs) > 0 {
		succeeded, err := agentutil.ParallelWorkerExec(ctx, "sending test log", logs, logger.Task(),
			func(log *testlog.TestLog) error {
				return errors.Wrapf(taskoutput.AppendTestLogForOutput(ctx, vTask, output.TestLogs, opts, log, conf.S3Usage), "uploading test log for virtual task '%s'", vTask.Id)
			},
		)
		if err != nil {
			return err
		}
		logger.Task().Infof(ctx, "Posting test logs succeeded for %d of %d logs.", succeeded, len(logs))
	}

	if len(logPaths) > 0 {
		return taskoutput.UploadTestLogFilesForOutput(ctx, taskoutput.TestLogFilesOptions{
			Dir:          resolveWorkingDirectory(conf.WorkDir, logDir),
			LogPaths:     logPaths,
			Task:         vTask,
			Output:       output.TestLogs,
			RedactorOpts: opts,
			Logger:       logger,
			S3Usage:      conf.S3Usage,
		})
	}
	return nil
}

// referencedTestLogPaths returns the unique test log paths referenced by the
// results' log info, excluding parsed test logs that are uploaded directly.
func referencedTestLogPaths(logs []testlog.TestLog, results []testresult.TestResult) []string {
	seen := make(map[string]bool, len(logs))
	for _, log := range logs {
		seen[log.Name] = true
	}
	var logPaths []string
	addPath := func(logPath string) {
		if logPath == "" || seen[logPath] {
			return
		}
		seen[logPath] = true
		logPaths = append(logPaths, logPath)
	}
	for _, r := range results {
		if r.LogInfo == nil {
			continue
		}
		addPath(r.LogInfo.LogName)
		for _, logPath := range r.LogInfo.LogsToMerge {
			addPath(utility.FromStringPtr(logPath))
		}
	}
	return logPaths
}

// uploadVirtualTaskTestResults uploads the test results in parquet format on
// behalf of the virtual task identified by info and returns their metadata.
func uploadVirtualTaskTestResults(ctx context.Context, conf *internal.TaskConfig, info testresult.TestResultsInfo, taskCreatedAt, testResultsCreatedAt time.Time, results []testresult.TestResult) (*apimodels.VirtualTaskTestResults, error) {
	output := conf.Task.TaskOutputInfo
	if output == nil {
		return nil, errors.New("runner task output info is not set")
	}
	switch output.TestResults.Version {
	case task.TestResultServiceCedar, task.TestResultServiceEvergreen:
	default:
		return nil, errors.New("invalid test results version")
	}

	// This is being called with AWS information from the runner task (e.g.
	// credentials, S3 bucket, S3 prefix) but using the virtual task's info (e.g.
	// task ID, build). That sharing is okay because the AWS information is not
	// task-specific.
	newResults := makeTestResults(taskCreatedAt, results)
	if err := uploadTestResultsParquet(ctx, conf.TaskOutput, *output, info, testResultsCreatedAt, newResults); err != nil {
		return nil, errors.Wrap(err, "uploading parquet test results")
	}

	failedCount, failedTests := computeTestResultsStats(newResults)
	return &apimodels.VirtualTaskTestResults{
		Stats: testresult.TaskTestResultsStats{
			TotalCount:  len(newResults),
			FailedCount: failedCount,
		},
		FailedSample: failedTests,
		CreatedAt:    testResultsCreatedAt,
	}, nil
}
