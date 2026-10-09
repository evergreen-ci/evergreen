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

// processVirtualTaskTestResultFiles parses the given test result file groups,
// uploads the test logs and test results to S3 on behalf of the virtual task
// identified by info, and returns the test result metadata to push to the
// completion route.
func processVirtualTaskTestResultFiles(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, info testresult.TestResultsInfo, taskCreatedAt, testResultsCreatedAt time.Time, groups []virtualTaskTestResultFileGroup) (*apimodels.VirtualTaskTestResults, error) {
	parseOpts := parseOptionsForVirtualTask(conf, info)

	logs, results, err := parseVirtualTaskTestResultFileGroups(ctx, parseOpts, logger, groups)
	if err != nil {
		return nil, errors.Wrap(err, "parsing test result files")
	}
	if len(results) == 0 {
		return nil, errors.New("no test results found in files")
	}

	if err = uploadVirtualTaskTestLogs(ctx, conf, logger, info, logs); err != nil {
		return nil, errors.Wrap(err, "uploading test logs")
	}

	return uploadVirtualTaskTestResults(ctx, conf, info, taskCreatedAt, testResultsCreatedAt, results)
}

// parseVirtualTaskTestResultFileGroups parses all test result file group into test logs and results.
func parseVirtualTaskTestResultFileGroups(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, groups []virtualTaskTestResultFileGroup) ([]testlog.TestLog, []testresult.TestResult, error) {
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
func parseVirtualTaskTestResultFileGroup(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, fileGroup virtualTaskTestResultFileGroup) ([]testlog.TestLog, []testresult.TestResult, error) {
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

// uploadVirtualTaskTestLogs uploads the given test logs on behalf of the virtual
// task identified by info.
func uploadVirtualTaskTestLogs(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, info testresult.TestResultsInfo, logs []testlog.TestLog) error {
	if len(logs) == 0 {
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

	succeeded, err := agentutil.ParallelWorkerExec(ctx, "sending test log", logs, logger.Task(),
		func(log *testlog.TestLog) error {
			return errors.Wrapf(taskoutput.AppendTestLogForOutput(ctx, vTask, output.TestLogs, opts, log, conf.S3Usage), "uploading test log for virtual task '%s'", vTask.Id)
		},
	)
	if err != nil {
		return err
	}

	logger.Task().Infof(ctx, "Posting test logs succeeded for %d of %d logs.", succeeded, len(logs))
	return nil
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
