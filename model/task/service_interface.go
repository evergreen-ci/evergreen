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

// maxQuarantinedTestsPerRecord caps how many quarantined tests are stored on a
// single test results record to keep the document under 16MB.
const maxQuarantinedTestsPerRecord = 40000

// maxTestSelectionCheckedTestsPerRecord caps how many tests checked by test
// selection are stored on a single test results record to keep the document
// under 16MB. Quarantined tests are also checked tests, so this matches the
// quarantined-tests cap.
const maxTestSelectionCheckedTestsPerRecord = 40000

// appendTestSelectionResultsUpdate returns the update pipeline that adds the
// given tests to a test results record's checked tests, appends the given
// tests to its quarantined-tests snapshot, and increments the snapshot count.
// Checked tests are deduplicated by the database. Once a list reaches its
// cap, further tests are dropped.
func appendTestSelectionResultsUpdate(info testresult.TestResultsInfo, checkedTests []string, quarantinedTests []testresult.QuarantinedTest) []bson.M {
	set := bson.M{
		testresult.TestResultsInfoKey: bson.M{"$ifNull": bson.A{"$" + testresult.TestResultsInfoKey, bson.M{"$literal": info}}},
		testresult.QuarantinedTestsCountKey: bson.M{"$add": bson.A{
			bson.M{"$ifNull": bson.A{"$" + testresult.QuarantinedTestsCountKey, 0}},
			len(quarantinedTests),
		}},
	}
	if len(checkedTests) > 0 {
		existing := bson.M{"$ifNull": bson.A{"$" + testresult.TestSelectionCheckedTestsKey, bson.A{}}}
		set[testresult.TestSelectionCheckedTestsKey] = bson.M{"$cond": bson.M{
			"if":   bson.M{"$gte": bson.A{bson.M{"$size": existing}, maxTestSelectionCheckedTestsPerRecord}},
			"then": existing,
			"else": bson.M{"$concatArrays": bson.A{existing, bson.M{"$slice": bson.A{
				bson.M{"$setDifference": bson.A{bson.M{"$literal": checkedTests}, existing}},
				bson.M{"$subtract": bson.A{maxTestSelectionCheckedTestsPerRecord, bson.M{"$size": existing}}},
			}}}},
		}}
	}
	if len(quarantinedTests) > 0 {
		set[testresult.QuarantinedTestsKey] = bson.M{"$slice": bson.A{
			bson.M{"$concatArrays": bson.A{
				bson.M{"$ifNull": bson.A{"$" + testresult.QuarantinedTestsKey, bson.A{}}},
				bson.M{"$literal": quarantinedTests},
			}},
			maxQuarantinedTestsPerRecord,
		}}
	}
	return []bson.M{{"$set": set}}
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
	// CheckedTestsFilter, if set, limits the tests checked by test selection
	// to the given test names in the database projection before they are
	// decoded. The returned tests are unordered.
	CheckedTestsFilter []string
}

// testResultsProjection returns the database projection for fetching test
// results metadata. When no fields are requested, it omits the potentially
// large test name lists that are only needed by specific callers.
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
	if _, ok := projection[testresult.TestSelectionCheckedTestsKey]; ok && getOpts.CheckedTestsFilter != nil {
		projection[testresult.TestSelectionCheckedTestsKey] = bson.M{"$setIntersection": bson.A{
			"$" + testresult.TestSelectionCheckedTestsKey,
			bson.M{"$literal": getOpts.CheckedTestsFilter},
		}}
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
