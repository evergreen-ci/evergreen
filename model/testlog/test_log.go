package testlog

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	TestLogCollection = "test_logs"

	maxDeleteCount = 100000
)

type TestLog struct {
	Id            string   `bson:"_id" json:"_id"`
	Name          string   `json:"name" bson:"name"`
	Task          string   `json:"task" bson:"task"`
	TaskExecution int      `json:"execution" bson:"execution"`
	Lines         []string `json:"lines" bson:"lines"`
}

var (
	TestLogIdKey            = bsonutil.MustHaveTag(TestLog{}, "Id")
	TestLogNameKey          = bsonutil.MustHaveTag(TestLog{}, "Name")
	TestLogTaskKey          = bsonutil.MustHaveTag(TestLog{}, "Task")
	TestLogTaskExecutionKey = bsonutil.MustHaveTag(TestLog{}, "TaskExecution")
	TestLogLinesKey         = bsonutil.MustHaveTag(TestLog{}, "Lines")
)

func FindOneTestLogById(id string) (*TestLog, error) {
	tl := &TestLog{}
	q := db.Query(bson.M{TestLogIdKey: id})
	err := db.FindOneQ(TestLogCollection, q, tl)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
}

// FindOneTestLog returns a TestLog, given the test's name, task id,
// and execution.
func FindOneTestLog(name, task string, execution int) (*TestLog, error) {
	tl := &TestLog{}
	q := db.Query(bson.M{
		TestLogNameKey:          name,
		TestLogTaskKey:          task,
		TestLogTaskExecutionKey: execution,
	})
	err := db.FindOneQ(TestLogCollection, q, tl)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
}

func findAllTestLogs(query db.Q) ([]TestLog, error) {
	var result []TestLog
	if err := db.FindAllQ(TestLogCollection, query, &result); err != nil {
		return nil, errors.Wrap(err, "finding test logs")
	}
	return result, nil
}

func DeleteTestLogsWithLimit(ctx context.Context, env evergreen.Environment, ts time.Time, limit int) (int, error) {
	if limit > maxDeleteCount {
		return 0, errors.Errorf("cannot delete more than %d documents in a single operation", maxDeleteCount)
	}

	docsToDelete, err := findAllTestLogs(db.Query(bson.M{TestLogIdKey: bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts).Hex()}}).WithFields(TestLogIdKey).Limit(limit))
	if err != nil {
		return 0, errors.Wrap(err, "getting docs to delete")
	}

	if len(docsToDelete) == 0 {
		return 0, nil
	}

	ops := make([]mongo.WriteModel, 0, len(docsToDelete))
	for _, doc := range docsToDelete {
		ops = append(ops, mongo.NewDeleteOneModel().SetFilter(bson.M{TestLogIdKey: doc.Id}))
	}

	res, err := env.DB().Collection(TestLogCollection).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}

// Insert inserts the TestLog into the database
func (tl *TestLog) Insert() error {
	tl.Id = mgobson.NewObjectId().Hex()
	if err := tl.Validate(); err != nil {
		return errors.Wrap(err, "invalid test log")
	}
	return errors.WithStack(db.Insert(TestLogCollection, tl))
}

// Validate makes sure the log will accessible in the database
// before the log itself is inserted. Returns an error if
// something is wrong.
func (tl *TestLog) Validate() error {
	switch {
	case tl.Name == "":
		return errors.New("test log requires a 'Name' field")
	case tl.Task == "":
		return errors.New("test log requires a 'Task' field")
	default:
		return nil
	}
}
