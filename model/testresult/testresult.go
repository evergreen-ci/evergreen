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
	return find(q)
}

// InsertByTaskIDAndExecution adds task metadata to a TestResult and then writes it to the database.
func (t *TestResult) InsertByTaskIDAndExecution(taskID string, execution int) error {
	if taskID == "" {
		return errors.New("Cannot insert test result with empty task ID")
	}
	t.TaskID = taskID
	t.Execution = execution
	return errors.Wrap(t.Insert(), "error inserting test result")
}

// InsertManyByTaskIDAndExecution adds task metadata to many TestResults and writes them to the database.
func InsertManyByTaskIDAndExecution(testResults []TestResult, taskID string, execution int) error {
	catcher := grip.NewSimpleCatcher()
	for _, t := range testResults {
		catcher.Add(t.InsertByTaskIDAndExecution(taskID, execution))
	}
	return catcher.Resolve()
}

// find returns all test results that satisfy the query. Returns an empty slice no tasks match.
func find(query db.Q) ([]TestResult, error) {
	tests := []TestResult{}
	err := db.FindAllQ(Collection, query, &tests)
	return tests, err
}

// Insert writes a test result to the database.
func (t *TestResult) Insert() error {
	return db.Insert(Collection, t)
}

// Aggregate runs an aggregation against the testresults collection.
func Aggregate(pipeline []bson.M, results interface{}) error {
	return db.Aggregate(
		Collection,
		pipeline,
		results)
}

// TestResultsPipeline is an aggregation pipeline for returning test results to the REST v2 API.
func TestResultsPipeline(id, filename, status string, limit, sort, execution int) []bson.M {
	// match test results
	pipeline := []bson.M{
		bson.M{
			"$match": bson.M{
				TaskIDKey:    id,
				ExecutionKey: execution,
			},
		},
	}

	// match status
	if status != "" {
		pipeline = append(pipeline, bson.M{
			"$match": bson.M{StatusKey: status},
		})
	}

	// sort
	sortOperator := "$gte"
	if sort < 0 {
		sortOperator = "$lte"
	}
	pipeline = append(pipeline,
		bson.M{
			"$match": bson.M{TestFileKey: bson.M{sortOperator: filename}}},
		bson.M{
			"$sort": bson.M{TestFileKey: 1},
		},
	)

	// limit
	if limit > 0 {
		pipeline = append(pipeline, bson.M{
			"$limit": limit,
		})
	}

	// project out task and execution
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			TaskIDKey:    0,
			ExecutionKey: 0,
		},
	})

	return pipeline
}
