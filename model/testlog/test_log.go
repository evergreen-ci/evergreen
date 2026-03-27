package testlog

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	TestLogCollection = "test_logs"
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

func FindOneTestLogById(ctx context.Context, id string) (*TestLog, error) {
	tl := &TestLog{}
	q := db.Query(bson.M{TestLogIdKey: id})
	err := db.FindOneQ(ctx, TestLogCollection, q, tl)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
}

// FindOneTestLog returns a TestLog, given the test's name, task id,
// and execution.
func FindOneTestLog(ctx context.Context, name, task string, execution int) (*TestLog, error) {
	tl := &TestLog{}
	q := db.Query(bson.M{
		TestLogNameKey:          name,
		TestLogTaskKey:          task,
		TestLogTaskExecutionKey: execution,
	})
	err := db.FindOneQ(ctx, TestLogCollection, q, tl)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
}

// Insert inserts the TestLog into the database
func (tl *TestLog) Insert(ctx context.Context) error {
	tl.Id = mgobson.NewObjectId().Hex()
	if err := tl.Validate(); err != nil {
		return errors.Wrap(err, "invalid test log")
	}
	return errors.WithStack(db.Insert(ctx, TestLogCollection, tl))
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
