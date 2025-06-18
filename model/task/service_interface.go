package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
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
	// TODO: DEVPROD-17978 Remove this function
	GetFailedTestSamples(context.Context, []Task, []string) ([]testresult.TaskTestResultsFailedSample, error)
	AppendTestResults(context.Context, testresult.DbTaskTestResults) error
	GetTaskTestResults(context.Context, []Task, []Task) ([]testresult.TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, []Task) (testresult.TaskTestResultsStats, error)
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

// FilterOptions represents the filtering arguments for fetching test results.
type FilterOptions struct {
	TestName            string
	ExcludeDisplayNames bool
	Statuses            []string
	GroupID             string
	Sort                []testresult.SortBy
	Limit               int
	Page                int
	BaseTasks           []Task
}
