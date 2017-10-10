package testresult

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	// Collection is the name of the test results collection in the database.
	Collection = "testresults"
)

// TestResult contains test data for a task.
type TestResult struct {
	ID        bson.ObjectId `bson:"_id" json:"id"`
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

// Insert writes a test result to the database.
func (t *TestResult) Insert() error {
	return db.Insert(Collection, t)
}

// FindByTaskIDAndExecution returns test results from the testresults collection for a given task.
func FindByTaskIDAndExecution(taskID string, execution int) ([]TestResult, error) {
	q := db.Query(bson.M{
		TaskIDKey:    taskID,
		ExecutionKey: execution,
	})
	return find(q)
}

// RemoveByTaskIDAndExecution removes test result from the testresults collection for a given task.

// find returns all test results that satisfy the query.
func find(query db.Q) ([]TestResult, error) {
	tests := []TestResult{}
	err := db.FindAllQ(Collection, query, &tests)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return tests, err
}

// FindOne returns one test result that satisfies the query.
func findOne(query db.Q) (*TestResult, error) {
	test := &TestResult{}
	err := db.FindOneQ(Collection, query, &test)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return test, err
}
