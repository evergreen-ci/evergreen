package taskoutput

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/pkg/errors"
)

// TestResultOutput is the versioned entry point for coordinating persistent
// storage of a task run's test result data.
type TestResultOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// AppendTestResults appends test results for the given task run.
func (o TestResultOutput) AppendTestResults(ctx context.Context, env evergreen.Environment, testResults []testresult.TestResult) error {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return errors.Wrap(err, "getting test result service")
	}

	return svc.AppendTestResults(ctx, testResults)
}

// GetMergedTaskTestResults returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedTaskTestResults(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions, getOpts *testresult.FilterOptions) (testresult.TaskTestResults, error) {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedTaskTestResults(ctx, ConvertOpts(taskOpts), getOpts)
}

// GetMergedTaskTestResultsStats returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedTaskTestResultsStats(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions) (testresult.TaskTestResultsStats, error) {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedTaskTestResultsStats(ctx, ConvertOpts(taskOpts))
}

// GetMergedFailedTestSample returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedFailedTestSample(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions) ([]string, error) {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return []string{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedFailedTestSample(ctx, ConvertOpts(taskOpts))
}

// GetFailedTestSamples returns failed test samples filtered as specified by
// the optional regex filters for each task specified.
func GetFailedTestSamples(ctx context.Context, env evergreen.Environment, taskOpts []TaskOptions, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	if len(taskOpts) == 0 {
		return nil, errors.New("must specify task options")
	}

	var allSamples []testresult.TaskTestResultsFailedSample
	for service, tasks := range groupTasksByService(taskOpts) {
		svc, err := testresult.GetServiceImpl(env, service)
		if err != nil {
			return nil, err
		}

		samples, err := svc.GetFailedTestSamples(ctx, ConvertOpts(tasks), regexFilters)
		if err != nil {
			return nil, err
		}

		allSamples = append(allSamples, samples...)
	}

	return allSamples, nil
}

func (o TestResultOutput) getTestResultService(env evergreen.Environment) (testresult.TestResultsService, error) {
	if o.Version == 0 {
		return testresult.NewCedarService(env), nil
	}
	return testresult.NewLocalService(env), nil
}

func groupTasksByService(taskOpts []TaskOptions) map[string][]TaskOptions {
	servicesToTasks := map[string][]TaskOptions{}
	for _, task := range taskOpts {
		servicesToTasks[task.ResultsService] = append(servicesToTasks[task.ResultsService], task)
	}
	return servicesToTasks
}

func ConvertOpts(taskOpts []TaskOptions) []testresult.TaskOptions {
	var opts []testresult.TaskOptions
	for _, taskOpt := range taskOpts {
		opts = append(opts, testresult.TaskOptions{TaskID: taskOpt.TaskID, Execution: taskOpt.Execution})
	}
	return opts
}
