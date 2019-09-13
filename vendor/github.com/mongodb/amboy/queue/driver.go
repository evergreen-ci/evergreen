package queue

import (
	"context"
	"time"

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

	Jobs(context.Context) <-chan amboy.Job
	Next(context.Context) amboy.Job

	Stats(context.Context) amboy.QueueStats
	JobStats(context.Context) <-chan amboy.JobStatusInfo
}

// MongoDBOptions is a struct passed to the NewMongo constructor to
// communicate mgoDriver specific settings about the driver's behavior
// and operation.
type MongoDBOptions struct {
	URI             string
	DB              string
	Priority        bool
	CheckWaitUntil  bool
	CheckDispatchBy bool
	SkipIndexBuilds bool
	Format          amboy.Format
	WaitInterval    time.Duration
	// TTL sets the number of seconds for a TTL index on the "info.created"
	// field. If set to zero, the TTL index will not be created and
	// and documents may live forever in the database.
	TTL time.Duration
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
		WaitInterval:    time.Second,
		Format:          amboy.BSON,
	}
}
