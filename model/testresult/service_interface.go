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

// testResultsService is an interface for fetching test results data from an
// underlying test results store.
type testResultsService interface {
	GetMergedTaskTestResults(context.Context, []TaskOptions, *FilterOptions) (TaskTestResults, error)
	GetMergedTaskTestResultsStats(context.Context, []TaskOptions) (TaskTestResultsStats, error)
	GetMergedFailedTestSample(context.Context, []TaskOptions) ([]string, error)
	GetFailedTestSamples(context.Context, []TaskOptions, []string) ([]TaskTestResultsFailedSample, error)
}

func getServiceImpl(env evergreen.Environment, service string) (testResultsService, error) {
	if service == "" {
		service = defaultService
	}

	switch service {
	case TestResultsServiceCedar:
		return newCedarService(env), nil
	case TestResultsServiceLocal:
		return newLocalService(env), nil
	default:
		return nil, errors.Errorf("unsupported test results service '%s'", service)
	}
}
