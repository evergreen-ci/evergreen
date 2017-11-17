package queue

import (
	"context"

	"github.com/mongodb/amboy"
)

// Driver describes the interface between a queue and an out of
// process persistence layer, like a database.
type Driver interface {
	Open(context.Context) error
	Close()

	Get(string) (amboy.Job, error)
	Put(amboy.Job) error
	Save(amboy.Job) error
	SaveStatus(amboy.Job, amboy.JobStatusInfo) error

	Jobs() <-chan amboy.Job
	Next(context.Context) amboy.Job

	Stats() amboy.QueueStats
	JobStats(context.Context) <-chan amboy.JobStatusInfo

	LockManager
}
