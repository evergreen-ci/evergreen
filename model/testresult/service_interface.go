package testresult

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// Valid test results services.
const (
	TestResultsServiceInMem = "in-mem"
	TestResultsServiceCedar = "cedar"
)

const defaultService = TestResultsServiceCedar

var serviceRegistry = map[string]testResultsServiceFactory{
	TestResultsServiceCedar: NewCedarTestResultsService,
}

type testResultsServiceFactory func(evergreen.Environment) testResultsService

type testResultsService interface {
	GetTaskTestResults(context.Context, TaskOptions, FilterOptions) (TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, TaskOptions) (TaskTestResultsStats, error)
	GetFailedTestSamples(context.Context, []TaskOptions, []string) ([]TaskTestResultsFailedSample, error)
}

func getService(env evergreen.Environment, service string) (testResultsService, error) {
	if service == "" {
		service = defaultService
	}

	svcFactory, ok := serviceRegistry[service]
	if !ok {
		return nil, errors.Errorf("unrecognized test results service '%s'", service)
	}

	return svcFactory(env), nil
}
