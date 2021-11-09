package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgobson "gopkg.in/mgo.v2/bson"
)

const TestLogCollection = "test_logs"

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

func DeleteTestLogsWithLimit(ctx context.Context, env evergreen.Environment, ts time.Time, limit int) (int, error) {
	if limit > 100*1000 {
		panic("cannot delete more than 100k documents in a single operation")
	}

	ops := make([]mongo.WriteModel, limit)
	for idx := 0; idx < limit; idx++ {
		ops[idx] = mongo.NewDeleteOneModel().SetFilter(bson.M{"_id": bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts).Hex()}})
	}

	res, err := env.DB().Collection(TestLogCollection).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}

// Insert inserts the TestLog into the database
func (self *TestLog) Insert() error {
	self.Id = mgobson.NewObjectId().Hex()
	if err := self.Validate(); err != nil {
		return errors.Wrap(err, "cannot insert invalid test log")
	}
	return errors.WithStack(db.Insert(TestLogCollection, self))
}

// Validate makes sure the log will accessible in the database
// before the log itself is inserted. Returns an error if
// something is wrong.
func (self *TestLog) Validate() error {
	switch {
	case self.Name == "":
		return errors.New("test log requires a 'Name' field")
	case self.Task == "":
		return errors.New("test log requires a 'Task' field")
	default:
		return nil
	}
}
