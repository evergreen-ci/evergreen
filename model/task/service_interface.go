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
	AppendTestSelectionResults(context.Context, testresult.DbTaskTestResults, TestSelectionResultsUpdate) error
	Get(context.Context, []Task, GetTaskTestResultsOptions) ([]testresult.TaskTestResults, error)
	GetTaskTestResultsStats(context.Context, []Task) (testresult.TaskTestResultsStats, error)
}

// TestSelectionResultsUpdate is a change to the test selection results
// recorded on a test results record.
type TestSelectionResultsUpdate struct {
	// QuarantinedTests are appended to the record's quarantined-tests
	// snapshot.
	QuarantinedTests []testresult.QuarantinedTest
}

// maxQuarantinedTestsPerRecord caps how many quarantined tests are stored on a
// single test results record to keep the document under 16MB.
const maxQuarantinedTestsPerRecord = 40000

// appendTestSelectionResultsUpdate returns the update document that appends the
// given tests to a test results record's quarantined-tests snapshot and
// increments the snapshot count.
func appendTestSelectionResultsUpdate(info testresult.TestResultsInfo, update TestSelectionResultsUpdate) bson.M {
	return bson.M{
		"$setOnInsert": bson.M{
			testresult.TestResultsInfoKey: info,
		},
		"$inc": bson.M{testresult.QuarantinedTestsCountKey: len(update.QuarantinedTests)},
		"$push": bson.M{
			testresult.QuarantinedTestsKey: bson.M{
				"$each":  update.QuarantinedTests,
				"$slice": maxQuarantinedTestsPerRecord,
			},
		},
	}
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

// testResultsProjection returns the database projection for fetching test
// results metadata.
func testResultsProjection(getOpts GetTaskTestResultsOptions) bson.M {
	if len(getOpts.Fields) == 0 {
		if getOpts.IncludeQuarantinedTests {
			return nil
		}
		return bson.M{testresult.QuarantinedTestsKey: 0}
	}

	projection := bson.M{}
	for _, field := range getOpts.Fields {
		projection[field] = 1
	}
	if getOpts.QuarantinedTestsLimit != nil {
		if *getOpts.QuarantinedTestsLimit == 0 {
			delete(projection, testresult.QuarantinedTestsKey)
		} else if *getOpts.QuarantinedTestsLimit > 0 {
			projection[testresult.QuarantinedTestsKey] = bson.M{"$slice": *getOpts.QuarantinedTestsLimit}
		}
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
