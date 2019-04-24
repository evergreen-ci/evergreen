package queue

import (
	"context"

	"github.com/mongodb/amboy"
)

// Driver describes the interface between a queue and an out of
// process persistence layer, like a database.
type Driver interface {
	ID() string
	Open(context.Context) error
	Close()

	Get(context.Context, string) (amboy.Job, error)
	Put(context.Context, amboy.Job) error
	Save(context.Context, amboy.Job) error
	SaveStatus(context.Context, amboy.Job, amboy.JobStatusInfo) error

	Jobs(context.Context) <-chan amboy.Job
	Next(context.Context) amboy.Job

	Stats(context.Context) amboy.QueueStats
	JobStats(context.Context) <-chan amboy.JobStatusInfo

	LockManager
}

// MongoDBOptions is a struct passed to the NewMgo constructor to
// communicate mgoDriver specific settings about the driver's behavior
// and operation.
type MongoDBOptions struct {
	URI             string
	DB              string
	Priority        bool
	CheckWaitUntil  bool
	SkipIndexBuilds bool
}

// DefaultMongoDBOptions constructs a new options object with default
// values: connecting to a MongoDB instance on localhost, using the
// "amboy" database, and *not* using priority ordering of jobs.
func DefaultMongoDBOptions() MongoDBOptions {
	return MongoDBOptions{
		URI:             "mongodb://localhost:27017",
		DB:              "amboy",
		Priority:        false,
		CheckWaitUntil:  true,
		SkipIndexBuilds: false,
	}
}
