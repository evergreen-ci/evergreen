package task

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"go.mongodb.org/mongo-driver/bson"
)

// TestResultsService is an interface for fetching test results data from an
// underlying test results store.
type TestResultsService interface {
	AppendTestResultMetadata(context.Context, []string, int, int, testresult.DbTaskTestResults) error
	AppendTestSelectionResults(context.Context, testresult.DbTaskTestResults, []string, []testresult.QuarantinedTest) error
	Get(context.Context, []Task, GetTaskTestResultsOptions) ([]testresult.TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, []Task) (testresult.TaskTestResultsStats, error)
}

// maxTestSelectionTestsPerRecord caps how many checked tests and how many
// quarantined tests are stored on a single test results record to keep the
// document under 16MB.
const maxTestSelectionTestsPerRecord = 40000

// appendTestSelectionResultsUpdate returns the update document that appends
// the given tests to a test results record's checked tests and
// quarantined-tests snapshot, and increments the snapshot count.
func appendTestSelectionResultsUpdate(info testresult.TestResultsInfo, checkedTests []string, quarantinedTests []testresult.QuarantinedTest) bson.M {
	push := bson.M{}
	if len(checkedTests) > 0 {
		push[testresult.TestSelectionCheckedTestsKey] = bson.M{
			"$each":  checkedTests,
			"$slice": maxTestSelectionTestsPerRecord,
		}
	}
	if len(quarantinedTests) > 0 {
		push[testresult.QuarantinedTestsKey] = bson.M{
			"$each":  quarantinedTests,
			"$slice": maxTestSelectionTestsPerRecord,
		}
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			testresult.TestResultsInfoKey: info,
		},
		"$inc": bson.M{testresult.QuarantinedTestsCountKey: len(quarantinedTests)},
	}
	if len(push) > 0 {
		update["$push"] = push
	}
	return update
}

// GetTaskTestResultsOptions configures how test result metadata is fetched.
type GetTaskTestResultsOptions struct {
	// Fields limits the returned test result metadata to the specified fields.
	Fields []string
	// IncludeQuarantinedTests includes quarantined test snapshots. The snapshots
	// are omitted by default because they can be large.
	IncludeQuarantinedTests bool
	// QuarantinedTestsLimit limits quarantined test snapshots in the database
	// projection before they are decoded.
	QuarantinedTestsLimit *int
}

// defaultTestResultsProjection returns the projection used when no fields
// are requested. It omits the potentially large test name lists that are
// only needed by specific callers.
func defaultTestResultsProjection(getOpts GetTaskTestResultsOptions) bson.M {
	projection := bson.M{testresult.TestSelectionCheckedTestsKey: 0}
	if !getOpts.IncludeQuarantinedTests {
		projection[testresult.QuarantinedTestsKey] = 0
	}
	return projection
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
