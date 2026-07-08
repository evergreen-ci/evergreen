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
	// Limit limits the number of test result rows returned after Skip. A
	// non-positive value returns all remaining rows.
	Limit int
	// Skip omits this many test result rows before returning results.
	Skip int
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

type testResultBounds struct {
	skip      int
	limit     int
	hasLimit  bool
	hasBounds bool
}

func newTestResultBounds(opts GetTaskTestResultsOptions) testResultBounds {
	return testResultBounds{
		skip:      opts.Skip,
		limit:     opts.Limit,
		hasLimit:  opts.Limit > 0,
		hasBounds: opts.Skip > 0 || opts.Limit > 0,
	}
}

func (b *testResultBounds) forTask(totalCount int) (int, int, bool) {
	if !b.hasBounds {
		return 0, 0, true
	}
	if b.skip >= totalCount {
		b.skip -= totalCount
		return 0, 0, false
	}

	skip := b.skip
	b.skip = 0
	if !b.hasLimit {
		return skip, 0, true
	}
	if b.limit <= 0 {
		return 0, 0, false
	}

	limit := min(b.limit, totalCount-skip)
	b.limit -= limit
	return skip, limit, true
}

func sliceTestResults(results []testresult.TestResult, skip, limit int) []testresult.TestResult {
	if skip >= len(results) {
		return nil
	}
	results = results[skip:]
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	return results
}
