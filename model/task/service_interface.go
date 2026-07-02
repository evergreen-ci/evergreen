package task

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/testresult"
)

// TestResultsService is an interface for fetching test results data from an
// underlying test results store.
type TestResultsService interface {
	AppendTestResultMetadata(context.Context, []string, int, int, testresult.DbTaskTestResults) error
	Get(context.Context, []Task, GetTaskTestResultsOptions) ([]testresult.TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, []Task) (testresult.TaskTestResultsStats, error)
}

// GetTaskTestResultsOptions configures how test result metadata is fetched.
type GetTaskTestResultsOptions struct {
	// Fields limits the returned test result metadata to the specified fields.
	Fields []string
	// IncludeQuarantinedTests includes quarantined test snapshots. The snapshots
	// are omitted by default because they can be large.
	IncludeQuarantinedTests bool
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

	IncludeQuarantinedTests bool
}
