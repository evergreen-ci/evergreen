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
