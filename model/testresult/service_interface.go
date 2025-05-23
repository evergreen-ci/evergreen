package testresult

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// Valid test results services.
const (
	TestResultsServiceLocal = "local"
	TestResultsServiceCedar = "cedar"
)

const defaultService = TestResultsServiceCedar

// TestResultsService is an interface for fetching test results data from an
// underlying test results store.
type TestResultsService interface {
	GetMergedTaskTestResults(context.Context, []TaskOptions, *FilterOptions) (TaskTestResults, error)
	GetMergedTaskTestResultsStats(context.Context, []TaskOptions) (TaskTestResultsStats, error)
	GetMergedFailedTestSample(context.Context, []TaskOptions) ([]string, error)
	GetFailedTestSamples(context.Context, []TaskOptions, []string) ([]TaskTestResultsFailedSample, error)
	AppendTestResults(context.Context, []TestResult) error
}

// GetServiceImpl fetches the specific test results service implementation based on the input service.
func GetServiceImpl(env evergreen.Environment, service string) (TestResultsService, error) {
	if service == "" {
		service = defaultService
	}

	switch service {
	case TestResultsServiceCedar:
		return NewCedarService(env), nil
	case TestResultsServiceLocal:
		return NewLocalService(env), nil
	default:
		return nil, errors.Errorf("unsupported test results service '%s'", service)
	}
}
