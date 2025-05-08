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
func (o TestResultOutput) AppendTestResults(ctx context.Context, testResults []testresult.TestResult) error {
	svc, err := o.getTestResultService()
	if err != nil {
		return errors.Wrap(err, "getting test result service")
	}

	return svc.AppendTestResults(ctx, testResults)
}

// GetMergedTaskTestResults returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedTaskTestResults(ctx context.Context, taskOpts []testresult.TaskOptions, getOpts *testresult.FilterOptions) (testresult.TaskTestResults, error) {
	svc, err := o.getTestResultService()
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedTaskTestResults(ctx, taskOpts, getOpts)
}

// GetMergedTaskTestResultsStats returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedTaskTestResultsStats(ctx context.Context, taskOpts []testresult.TaskOptions) (testresult.TaskTestResultsStats, error) {
	svc, err := o.getTestResultService()
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedTaskTestResultsStats(ctx, taskOpts)
}

// GetMergedFailedTestSample returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedFailedTestSample(ctx context.Context, taskOpts []testresult.TaskOptions) ([]string, error) {
	svc, err := o.getTestResultService()
	if err != nil {
		return []string{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetMergedFailedTestSample(ctx, taskOpts)
}

// GetFailedTestSamples returns failed test samples filtered as specified by
// the optional regex filters for each task specified.
func (o TestResultOutput) GetFailedTestSamples(ctx context.Context, taskOpts []testresult.TaskOptions, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	svc, err := o.getTestResultService()
	if err != nil {
		return []testresult.TaskTestResultsFailedSample{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetFailedTestSamples(ctx, taskOpts, regexFilters)
}

func (o TestResultOutput) getTestResultService() (testresult.TestResultsService, error) {
	return testresult.NewLocalService(evergreen.GetEnvironment()), nil
}
