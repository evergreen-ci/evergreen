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

// parseOptionsForVirtualTask returns the parse options for the virtual task
// identified by info. The virtual task is in the same project as the runner, so
// result file paths resolve against the runner's working directory.
func parseOptionsForVirtualTask(conf *internal.TaskConfig, info testresult.TestResultsInfo) testResultParseOptions {
	return testResultParseOptions{
		workDir:       conf.WorkDir,
		taskID:        info.TaskID,
		taskExecution: info.Execution,
	}
}

// processVirtualTaskTestResultFiles parses the given test result file specs,
// uploads the test logs and test results to S3 on behalf of the virtual task
// identified by info, and returns the test result metadata to push to the
// completion route.
func processVirtualTaskTestResultFiles(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, info testresult.TestResultsInfo, createTime, createdAt time.Time, specs []virtualTaskTestResultFile) (*apimodels.VirtualTaskTestResults, error) {
	parseOpts := parseOptionsForVirtualTask(conf, info)

	logs, results, err := parseVirtualTaskTestResultFiles(ctx, parseOpts, logger, specs)
	if err != nil {
		return nil, errors.Wrap(err, "parsing test result files")
	}
	if len(results) == 0 {
		return nil, errors.New("no test results found in files")
	}

	if err = uploadVirtualTaskTestLogs(ctx, conf, logger, info, logs); err != nil {
		return nil, errors.Wrap(err, "uploading test logs")
	}

	return uploadVirtualTaskTestResults(ctx, conf, info, createTime, createdAt, results)
}

// parseVirtualTaskTestResultFiles parses each spec into test logs and results.
func parseVirtualTaskTestResultFiles(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, specs []virtualTaskTestResultFile) ([]testlog.TestLog, []testresult.TestResult, error) {
	var (
		allLogs    []testlog.TestLog
		allResults []testresult.TestResult
	)
	catcher := grip.NewBasicCatcher()
	for _, spec := range specs {
		logs, results, err := parseVirtualTaskTestResultFile(ctx, opts, logger, spec)
		if err != nil {
			catcher.Wrapf(err, "parsing '%s' test results", spec.Type)
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

func parseVirtualTaskTestResultFile(ctx context.Context, opts testResultParseOptions, logger client.LoggerProducer, spec virtualTaskTestResultFile) ([]testlog.TestLog, []testresult.TestResult, error) {
	if len(spec.Files) == 0 {
		return nil, nil, errors.New("must specify at least one file")
	}

	switch spec.Type {
	case virtualTestResultTypeNative:
		var (
			logs    []testlog.TestLog
			results []testresult.TestResult
		)
		for _, file := range spec.Files {
			fileLogs, fileResults, err := parseNativeResults(opts, file)
			if err != nil {
				return nil, nil, err
			}
			logs = append(logs, fileLogs...)
			results = append(results, fileResults...)
		}
		return logs, results, nil
	case virtualTestResultTypeGo:
		patterns := make([]string, len(spec.Files))
		for i, file := range spec.Files {
			patterns[i] = resolveWorkingDirectory(opts.workDir, file)
		}
		files, err := globFiles(patterns...)
		if err != nil {
			return nil, nil, err
		}
		if len(files) == 0 {
			return nil, nil, errors.New("no files found to be parsed")
		}
		logs, results, _, err := parseTestOutputFiles(ctx, logger, opts, files)
		return logs, results, err
	case virtualTestResultTypeXUnit:
		cumulative, err := parseXUnitResults(ctx, opts, logger, spec.Files)
		if err != nil {
			return nil, nil, err
		}
		// The test results are only committed if their logs upload successfully,
		// so mark the logs as uploaded up front.
		for logIdx := range cumulative.logs {
			cumulative.markLogUploaded(logIdx)
		}
		logs := make([]testlog.TestLog, len(cumulative.logs))
		for i, log := range cumulative.logs {
			logs[i] = *log
		}
		return logs, cumulative.tests, nil
	default:
		return nil, nil, errors.Errorf("unrecognized test result type '%s'", spec.Type)
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
	vtask := task.Task{
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
			return taskoutput.AppendTestLogForOutput(ctx, vtask, output.TestLogs, opts, log, conf.S3Usage)
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
func uploadVirtualTaskTestResults(ctx context.Context, conf *internal.TaskConfig, info testresult.TestResultsInfo, createTime, createdAt time.Time, results []testresult.TestResult) (*apimodels.VirtualTaskTestResults, error) {
	output := conf.Task.TaskOutputInfo
	if output == nil {
		return nil, errors.New("runner task output info is not set")
	}
	switch output.TestResults.Version {
	case task.TestResultServiceCedar, task.TestResultServiceEvergreen:
	default:
		return nil, errors.New("invalid test results version")
	}

	vtask := task.Task{CreateTime: createTime}
	newResults := makeTestResults(&vtask, results)
	if err := uploadTestResultsParquet(ctx, conf.TaskOutput, *output, info, createdAt, newResults); err != nil {
		return nil, errors.Wrap(err, "uploading parquet test results")
	}

	failedCount, failedTests := computeTestResultsStats(newResults)
	return &apimodels.VirtualTaskTestResults{
		Stats: testresult.TaskTestResultsStats{
			TotalCount:  len(newResults),
			FailedCount: failedCount,
		},
		FailedSample: failedTests,
		CreatedAt:    createdAt,
	}, nil
}
