package task

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/testresult"
)

// TestResultsService is an interface for fetching test results data from an
// underlying test results store.
type TestResultsService interface {
	AppendTestResultMetadata(context.Context, []string, int, int, testresult.DbTaskTestResults) error
	GetTaskTestResults(context.Context, []Task) ([]testresult.TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, []Task) (testresult.TaskTestResultsStats, error)
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
