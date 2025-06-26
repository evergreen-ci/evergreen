package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
)

// Valid test results services.
const (
	TestResultsServiceEvergreen = "evergreen"
	TestResultsServiceCedar     = "cedar"
)

const defaultService = TestResultsServiceCedar

// TestResultsService is an interface for fetching test results data from an
// underlying test results store.
type TestResultsService interface {
	// TODO: DEVPROD-17978 Remove this function
	GetFailedTestSamples(context.Context, []Task, []string) ([]testresult.TaskTestResultsFailedSample, error)
	AppendTestResults(context.Context, []testresult.TestResult) error
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
	case TestResultsServiceEvergreen:
		return NewEvergreenService(env), nil
	default:
		return NewLocalService(env), nil
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
