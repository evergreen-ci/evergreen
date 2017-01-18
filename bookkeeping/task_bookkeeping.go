package bookkeeping

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/tychoish/grip"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var _ fmt.Stringer = nil

const (
	DefaultGuessDur           = time.Minute * 20
	TaskBookkeepingCollection = "task_bk"
)

type TaskBookkeeping struct {
	// standard object id
	Id bson.ObjectId `bson:"_id"`

	// info that tasks with the same guessed duration will share
	Name         string `bson:"name"`
	BuildVariant string `bson:"build_variant"`
	HostType     string `bson:"host_type"` // may change to an enum once Host.HostType changes
	Project      string `bson:"branch"`

	// the duration we expect the task to take
	ExpectedDuration time.Duration `bson:"expected_duration"`

	// the number of times this task - as defined by the
	// buildvariant and task name - has been started
	NumStarted int64 `bson:"num_started"`
}

/************************************************************
Helper functions to reduce boilerplate
************************************************************/

// finds a bookkeeping entry matching the specified interface
func findOneTaskBk(matcher interface{}, selector interface{}) (*TaskBookkeeping, error) {

	// establish a database connection
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("Error establishing database connection: %+v", err)
		return nil, err
	}

	// make sure the function is closed when the function exits
	defer session.Close()

	// query for the bookkeeping entry
	taskBk := &TaskBookkeeping{}
	err = db.C(TaskBookkeepingCollection).Find(matcher).Select(selector).One(taskBk)

	// no entry was found
	if err == mgo.ErrNotFound {
		return nil, nil
	}

	// failure
	if err != nil {
		grip.Errorf("Unexpected error retrieving task bookkeeping entry from database: %+v",
			err)
		return nil, err
	}

	// success
	return taskBk, nil
}

// upsert a single bookkeeping entry
func upsertOneTaskBk(matcher interface{}, update interface{}) error {

	// establish a database connection
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		grip.Errorf("Error establishing database connection: %+v", err)
		return err
	}

	// make sure the session is closed when the function exits
	defer session.Close()

	// update the bookkeeping entry
	_, err = db.C(TaskBookkeepingCollection).Upsert(matcher, update)
	return err
}

// update the expected duration that we expect the given task to take when run on the
// given host
func UpdateExpectedDuration(t *task.Task, timeTaken time.Duration) error {
	matcher := bson.M{
		"name":          t.DisplayName,
		"build_variant": t.BuildVariant,
		"branch":        t.Project,
	}
	taskBk, err := findOneTaskBk(matcher, bson.M{})
	if err != nil {
		return err
	}
	var averageTaskDuration time.Duration

	if taskBk == nil {
		averageTaskDuration = timeTaken
	} else {
		averageTime := ((taskBk.ExpectedDuration.Nanoseconds() * taskBk.NumStarted) + timeTaken.Nanoseconds()) / (taskBk.NumStarted + 1)
		averageTaskDuration = time.Duration(averageTime)
	}

	// for now, we are just using the duration of the last comparable task ran as the
	// guess for upcoming tasks

	update := bson.M{
		"$set": bson.M{"expected_duration": averageTaskDuration},
		"$inc": bson.M{"num_started": 1},
	}

	return upsertOneTaskBk(matcher, update)
}
