package task

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
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
	// PrevCheckedTests are the checked tests that the record is expected to
	// have. If the record has different ones, the update fails with
	// errTestSelectionResultsConflict.
	PrevCheckedTests *testresult.TestNameFilter
	// CheckedTests, if set, replaces the record's checked tests.
	CheckedTests *testresult.TestNameFilter
	// QuarantinedTests are appended to the record's quarantined-tests
	// snapshot.
	QuarantinedTests []testresult.QuarantinedTest
}

// errTestSelectionResultsConflict means the record's checked tests changed
// since they were read.
var errTestSelectionResultsConflict = errors.New("test selection results were concurrently modified")

// maxQuarantinedTestsPerRecord caps how many quarantined tests are stored on a
// single test results record to keep the document under 16MB.
const maxQuarantinedTestsPerRecord = 40000

// appendTestSelectionResultsQuery returns the query that matches the test
// results record with the given ID only if its checked tests are the update's
// previous checked tests.
func appendTestSelectionResultsQuery(id any, update TestSelectionResultsUpdate) bson.M {
	query := bson.M{IdKey: id}
	if update.CheckedTests == nil {
		return query
	}
	if update.PrevCheckedTests == nil {
		query[testresult.TestSelectionCheckedTestsKey] = bson.M{"$exists": false}
	} else {
		query[bsonutil.GetDottedKeyName(testresult.TestSelectionCheckedTestsKey, testresult.TestNameFilterBitsKey)] = update.PrevCheckedTests.Bits
	}
	return query
}

// appendTestSelectionResultsUpdate returns the update document that replaces a
// test results record's checked tests, appends the given tests to its
// quarantined-tests snapshot, and increments the snapshot count.
func appendTestSelectionResultsUpdate(info testresult.TestResultsInfo, update TestSelectionResultsUpdate) bson.M {
	doc := bson.M{
		"$setOnInsert": bson.M{
			testresult.TestResultsInfoKey: info,
		},
		"$inc": bson.M{testresult.QuarantinedTestsCountKey: len(update.QuarantinedTests)},
	}
	if len(update.QuarantinedTests) > 0 {
		doc["$push"] = bson.M{
			testresult.QuarantinedTestsKey: bson.M{
				"$each":  update.QuarantinedTests,
				"$slice": maxQuarantinedTestsPerRecord,
			},
		}
	}
	if update.CheckedTests != nil {
		doc["$set"] = bson.M{testresult.TestSelectionCheckedTestsKey: update.CheckedTests}
	}
	return doc
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
// results metadata. When no fields are requested, it omits the test selection
// results that are only needed by specific callers.
func testResultsProjection(getOpts GetTaskTestResultsOptions) bson.M {
	if len(getOpts.Fields) == 0 {
		projection := bson.M{testresult.TestSelectionCheckedTestsKey: 0}
		if !getOpts.IncludeQuarantinedTests {
			projection[testresult.QuarantinedTestsKey] = 0
		}
		return projection
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
