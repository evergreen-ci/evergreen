package testresult

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// Collection is the name of the test results collection in the database.
	Collection = "testresults"
)

// TestResult contains test data for a task.
type TestResult struct {
	ID        bson.ObjectId `bson:"_id,omitempty" json:"id"`
	Status    string        `json:"status" bson:"status"`
	TestFile  string        `json:"test_file" bson:"test_file"`
	URL       string        `json:"url" bson:"url,omitempty"`
	URLRaw    string        `json:"url_raw" bson:"url_raw,omitempty"`
	LogID     string        `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum   int           `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode  int           `json:"exit_code" bson:"exit_code"`
	StartTime float64       `json:"start" bson:"start"`
	EndTime   float64       `json:"end" bson:"end"`

	// Together, TaskID and Execution identify the task which created this TestResult
	TaskID    string `bson:"task_id" json:"task_id"`
	Execution int    `bson:"task_execution" json:"task_execution"`

	// LogRaw is not persisted to the database
	LogRaw string `json:"log_raw" bson:"log_raw,omitempty"`
}

var (
	// BSON fields for the task struct
	IDKey        = bsonutil.MustHaveTag(TestResult{}, "ID")
	StatusKey    = bsonutil.MustHaveTag(TestResult{}, "Status")
	LineNumKey   = bsonutil.MustHaveTag(TestResult{}, "LineNum")
	TestFileKey  = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	URLKey       = bsonutil.MustHaveTag(TestResult{}, "URL")
	LogIDKey     = bsonutil.MustHaveTag(TestResult{}, "LogID")
	URLRawKey    = bsonutil.MustHaveTag(TestResult{}, "URLRaw")
	ExitCodeKey  = bsonutil.MustHaveTag(TestResult{}, "ExitCode")
	StartTimeKey = bsonutil.MustHaveTag(TestResult{}, "StartTime")
	EndTimeKey   = bsonutil.MustHaveTag(TestResult{}, "EndTime")
	TaskIDKey    = bsonutil.MustHaveTag(TestResult{}, "TaskID")
	ExecutionKey = bsonutil.MustHaveTag(TestResult{}, "Execution")
)

// FindByTaskIDAndExecution returns test results from the testresults collection for a given task.
func FindByTaskIDAndExecution(taskID string, execution int) ([]TestResult, error) {
	q := db.Query(bson.M{
		TaskIDKey:    taskID,
		ExecutionKey: execution,
	}).Project(bson.M{
		TaskIDKey:    0,
		ExecutionKey: 0,
	})
	return Find(q)
}

func ByTaskIDs(ids []string) db.Q {
	return db.Query(bson.M{
		TaskIDKey: bson.M{
			"$in": ids,
		},
	})
}

// find returns all test results that satisfy the query. Returns an empty slice no tasks match.
func Find(query db.Q) ([]TestResult, error) {
	tests := []TestResult{}
	err := db.FindAllQ(Collection, query, &tests)
	return tests, err
}

// Insert writes a test result to the database.
func (t *TestResult) Insert() error {
	return db.Insert(Collection, t)
}

func InsertMany(results []TestResult) error {
	docs := make([]interface{}, len(results))
	catcher := grip.NewSimpleCatcher()
	for idx, result := range results {
		if result.TaskID == "" {
			catcher.Add(errors.New("Cannot insert test result with empty task ID"))
		}
		docs[idx] = results[idx]
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	return errors.WithStack(db.InsertMany(Collection, docs...))
}

// Aggregate runs an aggregation against the testresults collection.
func Aggregate(pipeline []bson.M, results interface{}) error {
	return db.Aggregate(
		Collection,
		pipeline,
		results)
}

// TestResultsQuery is a query for returning test results to the REST v2 API.
func TestResultsQuery(taskId, testId, status string, limit, execution int) db.Q {
	match := bson.M{
		TaskIDKey:    taskId,
		ExecutionKey: execution,
	}
	if status != "" {
		match[StatusKey] = status
	}
	if testId != "" {
		match[IDKey] = bson.M{"$gte": bson.ObjectId(testId)}
	}

	q := db.Query(match).Sort([]string{IDKey}).Project(bson.M{
		TaskIDKey:    0,
		ExecutionKey: 0,
	})

	if limit > 0 {
		q = q.Limit(limit)
	}

	return q
}
