package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	err := db.FindOne(
		TestLogCollection,
		bson.M{
			TestLogIdKey: id,
		},
		db.NoProjection,
		db.NoSort,
		tl,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
}

// FindOneTestLog returns a TestLog, given the test's name, task id,
// and execution.
func FindOneTestLog(name, task string, execution int) (*TestLog, error) {
	tl := &TestLog{}
	err := db.FindOne(
		TestLogCollection,
		bson.M{
			TestLogNameKey:          name,
			TestLogTaskKey:          task,
			TestLogTaskExecutionKey: execution,
		},
		db.NoProjection,
		db.NoSort,
		tl,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return tl, errors.WithStack(err)
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

// URL returns the path to access the log based on its current fields.
// Does not error if fields are not set.
func (self *TestLog) URL() string {
	settings, err := evergreen.GetConfig()
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":   "error getting config",
			"operation": "set test url",
			"task":      self.Task,
		}))
	}
	return fmt.Sprintf("%v/test_log/%v/%v/%v",
		settings.Ui.Url,
		self.Task,
		self.TaskExecution,
		self.Name,
	)
}
