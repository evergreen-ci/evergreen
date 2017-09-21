package testresult

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	collection = "testresults"
)

// TestResult contains test data for a task.
type TestResult struct {
	ID        bson.ObjectId `bson:"_id" json:"_id"`
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
	testResultStatusKey    = bsonutil.MustHaveTag(TestResult{}, "Status")
	testResultLineNumKey   = bsonutil.MustHaveTag(TestResult{}, "LineNum")
	testResultTestFileKey  = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	testResultURLKey       = bsonutil.MustHaveTag(TestResult{}, "URL")
	testResultLogIDKey     = bsonutil.MustHaveTag(TestResult{}, "LogID")
	testResultURLRawKey    = bsonutil.MustHaveTag(TestResult{}, "URLRaw")
	testResultExitCodeKey  = bsonutil.MustHaveTag(TestResult{}, "ExitCode")
	testResultStartTimeKey = bsonutil.MustHaveTag(TestResult{}, "StartTime")
	testResultEndTimeKey   = bsonutil.MustHaveTag(TestResult{}, "EndTime")
	testResultTaskIDKey    = bsonutil.MustHaveTag(TestResult{}, "TaskID")
	testResultExecutionKey = bsonutil.MustHaveTag(TestResult{}, "Execution")
)

// Insert writes a test result to the database.
func (t *TestResult) Insert() error {
	return db.Insert(collection, t)
}

// ByTaskIDAndExecution creates a query to return test results from the testresults collection for a given task.
func ByTaskIDAndExecution(taskID string, execution int) db.Q {
	return db.Query(bson.M{
		testResultTaskIDKey:    taskID,
		testResultExecutionKey: execution,
	})
}

// Find returns all test results that satisfy the query.
func Find(query db.Q) ([]TestResult, error) {
	tests := []TestResult{}
	err := db.FindAllQ(collection, query, &tests)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return tests, err
}

// FindOne returns one test result that satisfies the query.
func FindOne(query db.Q) (*TestResult, error) {
	test := &TestResult{}
	err := db.FindOneQ(collection, query, &test)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return test, err
}
