package testresult

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	// Collection is the name of the test results collection in the database.
	Collection = "testresults"
)

// TestResult contains test data for a task.
type TestResult struct {
	ID              mgobson.ObjectId `bson:"_id,omitempty" json:"id"`
	Status          string           `json:"status" bson:"status"`
	TestFile        string           `json:"test_file" bson:"test_file"`
	DisplayTestName string           `json:"display_test_name" bson:"display_test_name"`
	GroupID         string           `json:"group_id,omitempty" bson:"group_id,omitempty"`
	URL             string           `json:"url" bson:"url,omitempty"`
	URLRaw          string           `json:"url_raw" bson:"url_raw,omitempty"`
	LogID           string           `json:"log_id,omitempty" bson:"log_id,omitempty"`
	LineNum         int              `json:"line_num,omitempty" bson:"line_num,omitempty"`
	ExitCode        int              `json:"exit_code" bson:"exit_code"`
	StartTime       float64          `json:"start" bson:"start"`
	EndTime         float64          `json:"end" bson:"end"`

	// Together, TaskID and Execution identify the task which created this TestResult
	TaskID    string `bson:"task_id" json:"task_id"`
	Execution int    `bson:"task_execution" json:"task_execution"`

	// LogRaw is not persisted to the database
	LogRaw string `json:"log_raw" bson:"log_raw,omitempty"`

	// Denormalize TestResults to add fields from enclosing tasks. This
	// facilitates faster historical test stats  aggregations.
	Project              string    `bson:"branch" json:"branch"`
	BuildVariant         string    `bson:"build_variant" json:"build_variant"`
	DistroId             string    `bson:"distro" json:"distro"`
	Requester            string    `bson:"r" json:"r"`
	DisplayName          string    `bson:"display_name" json:"display_name"`
	ExecutionDisplayName string    `bson:"execution_display_name,omitempty" json:"execution_display_name,omitempty"`
	TaskCreateTime       time.Time `bson:"task_create_time" json:"task_create_time"`

	TestStartTime time.Time `json:"test_start_time" bson:"test_start_time"`
	TestEndTime   time.Time `json:"test_end_time" bson:"test_end_time"`
}

var (
	// BSON fields for the task struct
	IDKey              = bsonutil.MustHaveTag(TestResult{}, "ID")
	StatusKey          = bsonutil.MustHaveTag(TestResult{}, "Status")
	LineNumKey         = bsonutil.MustHaveTag(TestResult{}, "LineNum")
	TestFileKey        = bsonutil.MustHaveTag(TestResult{}, "TestFile")
	DisplayTestNameKey = bsonutil.MustHaveTag(TestResult{}, "DisplayTestName")
	GroupIDKey         = bsonutil.MustHaveTag(TestResult{}, "GroupID")
	URLKey             = bsonutil.MustHaveTag(TestResult{}, "URL")
	LogIDKey           = bsonutil.MustHaveTag(TestResult{}, "LogID")
	URLRawKey          = bsonutil.MustHaveTag(TestResult{}, "URLRaw")
	ExitCodeKey        = bsonutil.MustHaveTag(TestResult{}, "ExitCode")
	StartTimeKey       = bsonutil.MustHaveTag(TestResult{}, "StartTime")
	EndTimeKey         = bsonutil.MustHaveTag(TestResult{}, "EndTime")
	TaskIDKey          = bsonutil.MustHaveTag(TestResult{}, "TaskID")
	ExecutionKey       = bsonutil.MustHaveTag(TestResult{}, "Execution")

	ProjectKey      = bsonutil.MustHaveTag(TestResult{}, "Project")
	BuildVariantKey = bsonutil.MustHaveTag(TestResult{}, "BuildVariant")
	DistroIdKey     = bsonutil.MustHaveTag(TestResult{}, "DistroId")
	RequesterKey    = bsonutil.MustHaveTag(TestResult{}, "Requester")
	DisplayNameKey  = bsonutil.MustHaveTag(TestResult{}, "DisplayName")

	ExecutionDisplayNameKey = bsonutil.MustHaveTag(TestResult{}, "ExecutionDisplayName")
	TaskCreateTimeKey       = bsonutil.MustHaveTag(TestResult{}, "TaskCreateTime")
)

func (t *TestResult) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(t) }
func (t *TestResult) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, t) }

// Count returns the number of testresults that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// FindByTaskIDAndExecution returns test results from the testresults collection for a given task.
func FindByTaskIDAndExecution(taskID string, execution int) ([]TestResult, error) {
	return Find(FilterByTaskIDAndExecution(taskID, execution))
}

func FilterByTaskIDAndExecution(taskID string, execution int) db.Q {
	return db.Query(bson.M{
		TaskIDKey:    taskID,
		ExecutionKey: execution,
	})
}

func ByTaskIDs(ids []string) db.Q {
	return db.Query(bson.M{
		TaskIDKey: bson.M{
			"$in": ids,
		},
	})
}

// find returns all test results that satisfy the query. Returns an empty slice if no tasks match.
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
func TestResultsQuery(taskIds []string, testId, testName, status string, limit, execution int) db.Q {
	match := bson.M{
		TaskIDKey:    bson.M{"$in": taskIds},
		ExecutionKey: execution,
	}
	if status != "" {
		match[StatusKey] = status
	}
	if testName != "" {
		match[TestFileKey] = testName
	}
	if testId != "" {
		match[IDKey] = bson.M{"$gte": mgobson.ObjectId(testId)}
	}

	q := db.Query(match).Project(bson.M{
		TaskIDKey:    0,
		ExecutionKey: 0,
	})

	// Don't sort if unlimited EVG-13965.
	if limit > 0 {
		q = q.Limit(limit)
		q = q.Sort([]string{IDKey})
	}

	return q
}

func TestResultCount(taskIds []string, testName string, statuses []string, execution int) (int, error) {
	filter := bson.M{
		TaskIDKey:    bson.M{"$in": taskIds},
		ExecutionKey: execution,
	}
	if len(statuses) > 0 {
		filter[StatusKey] = bson.M{"$in": statuses}
	}
	if testName != "" {
		filter[TestFileKey] = bson.M{"$regex": testName, "$options": "i"}
	}
	q := db.Query(filter)
	return Count(q)
}

// TestResultsFilterSortPaginateOpts contains filtering, sorting and pagination options for TestResults.
type TestResultsFilterSortPaginateOpts struct {
	Execution int
	GroupID   string
	Limit     int
	Page      int
	SortBy    string
	SortDir   int
	Statuses  []string
	// TaskIDs is the only required option.
	TaskIDs  []string
	TestID   string
	TestName string
}

// TestResultsFilterSortPaginate is a query for returning test results from supplied TaskIDs.
func TestResultsFilterSortPaginate(opts TestResultsFilterSortPaginateOpts) ([]TestResult, error) {
	tests := []TestResult{}
	match := bson.M{
		TaskIDKey:    bson.M{"$in": opts.TaskIDs},
		ExecutionKey: opts.Execution,
	}
	if len(opts.Statuses) > 0 {
		match[StatusKey] = bson.M{"$in": opts.Statuses}
	}
	if opts.GroupID != "" {
		match[GroupIDKey] = opts.GroupID
	}

	if opts.TestID != "" {
		match[IDKey] = bson.M{"$gte": mgobson.ObjectId(opts.TestID)}
	}

	pipeline := []bson.M{
		{"$match": match},
	}

	project := bson.M{
		"duration":         bson.M{"$subtract": []string{"$" + EndTimeKey, "$" + StartTimeKey}},
		DisplayTestNameKey: 1,
		EndTimeKey:         1,
		ExitCodeKey:        1,
		GroupIDKey:         1,
		LineNumKey:         1,
		LogIDKey:           1,
		StartTimeKey:       1,
		StatusKey:          1,
		TaskIDKey:          1,
		TestFileKey:        1,
		URLKey:             1,
		URLRawKey:          1,
	}

	pipeline = append(pipeline, bson.M{"$project": project})

	if opts.TestName != "" {
		matchTestName := bson.M{"$match": bson.M{TestFileKey: bson.M{"$regex": opts.TestName, "$options": "i"}}}
		pipeline = append(pipeline, matchTestName)
	}

	sort := bson.D{}
	if opts.SortBy != "" {
		sort = append(sort, primitive.E{Key: opts.SortBy, Value: opts.SortDir})
	} else if opts.Limit > 0 { // Don't sort TaskID if unlimited EVG-13965.
		sort = append(sort, primitive.E{Key: TaskIDKey, Value: 1})
	}

	sort = append(sort, primitive.E{Key: "_id", Value: 1})
	pipeline = append(pipeline, bson.M{"$sort": sort})

	if opts.Page > 0 {
		pipeline = append(pipeline, bson.M{"$skip": opts.Page * opts.Limit})
	}

	if opts.Limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})
	}
	err := db.Aggregate(Collection, pipeline, &tests)
	if err != nil {
		return nil, err
	}

	return tests, nil
}

func DeleteWithLimit(ctx context.Context, env evergreen.Environment, ts time.Time, limit int) (int, error) {
	if limit > 100*1000 {
		panic("cannot delete more than 100k documents in a single operation")
	}

	ops := make([]mongo.WriteModel, limit)
	for idx := 0; idx < limit; idx++ {
		ops[idx] = mongo.NewDeleteOneModel().SetFilter(bson.M{"_id": bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts)}})
	}

	res, err := env.DB().Collection(Collection).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}
