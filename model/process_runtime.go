package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	RuntimesCollection = "process_runtimes"
)

// ProcessRuntime tracks the most recent success ping by
// a given MCI process by storing it in mongodb.
// Id is a package name (see globals.go), FinishedAt is a time
// representing the most recent completion of that process,
// and Runtime is the duration of time the process took to run
type ProcessRuntime struct {
	Id         string        `bson:"_id"         json:"id"`
	FinishedAt time.Time     `bson:"finished_at" json:"finished_at"`
	Runtime    time.Duration `bson:"runtime"     json:"runtime"`
}

var (
	ProcRuntimeIdKey         = bsonutil.MustHaveTag(ProcessRuntime{}, "Id")
	ProcRuntimeFinishedAtKey = bsonutil.MustHaveTag(ProcessRuntime{},
		"FinishedAt")
	ProcRuntimeRuntimeKey = bsonutil.MustHaveTag(ProcessRuntime{}, "Runtime")
)

/******************************************************
Find
******************************************************/

func FindAllProcessRuntimes(query interface{},
	projection interface{}) ([]ProcessRuntime, error) {
	runtimes := []ProcessRuntime{}
	err := db.FindAll(
		RuntimesCollection,
		query,
		projection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&runtimes,
	)
	return runtimes, err
}

func FindOneProcessRuntime(query interface{},
	projection interface{}) (*ProcessRuntime, error) {
	runtime := &ProcessRuntime{}
	err := db.FindOne(
		RuntimesCollection,
		query,
		projection,
		db.NoSort,
		runtime,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return runtime, err
}

// Finds all runtimes that were updated before (less than) given time.
func FindAllLateProcessRuntimes(cutoff time.Time) ([]ProcessRuntime, error) {
	return FindAllProcessRuntimes(
		bson.M{
			ProcRuntimeFinishedAtKey: bson.M{
				"$lt": cutoff,
			},
		},
		db.NoProjection,
	)
}

// Finds a process runtime by Id
func FindProcessRuntime(id string) (*ProcessRuntime, error) {
	return FindOneProcessRuntime(
		bson.M{
			ProcRuntimeIdKey: id,
		},
		db.NoProjection,
	)
}

// Returns list of all process runtime entries
func FindEveryProcessRuntime() ([]ProcessRuntime, error) {
	return FindAllProcessRuntimes(
		bson.M{},
		db.NoProjection,
	)
}

/******************************************************
Update
******************************************************/

func UpsertOneProcessRuntime(query interface{}, update interface{}) error {
	info, err := db.Upsert(
		RuntimesCollection,
		query,
		update,
	)
	if err != nil {
		return err
	}

	grip.InfoWhenf(info.UpsertedId != nil, "Added '%s' process to ProcessRuntime  db",
		info.UpsertedId)

	return nil
}

// Updates a process runtime to set recent_success to the current time.
// If no process with the given name exists, create it.
// Parameter "processName" should be a constant "mci package" name from globals.go
func SetProcessRuntimeCompleted(processName string,
	runtime time.Duration) error {
	return UpsertOneProcessRuntime(
		bson.M{
			ProcRuntimeIdKey: processName,
		},
		bson.M{
			"$set": bson.M{
				ProcRuntimeFinishedAtKey: time.Now(),
				ProcRuntimeRuntimeKey:    runtime,
			},
		},
	)
}
