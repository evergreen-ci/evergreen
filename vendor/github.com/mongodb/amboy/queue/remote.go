package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Remote queues extend the queue interface to allow a
// pluggable-storage backend, or "driver"
type Remote interface {
	amboy.Queue
	SetDriver(Driver) error
	Driver() Driver
}

// RemoteUnordered are queues that use a Driver as backend for job
// storage and processing and do not impose any additional ordering
// beyond what's provided by the driver.
type remoteUnordered struct {
	*remoteBase
}

// NewRemoteUnordered returns a queue that has been initialized with a
// local worker pool Runner instance of the specified size.
func NewRemoteUnordered(size int) Remote {
	q := &remoteUnordered{
		remoteBase: newRemoteBase(),
	}

	grip.Error(q.SetRunner(pool.NewLocalWorkers(size, q)))

	grip.Infof("creating new remote job queue with %d workers", size)

	return q
}

// Next returns a Job from the queue. Returns a nil Job object if the
// context is canceled. The operation is blocking until an
// undispatched, unlocked job is available. This operation takes a job
// lock.
func (q *remoteUnordered) Next(ctx context.Context) amboy.Job {
	var err error

	start := time.Now()
	count := 0
	for {
		count++
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			if job == nil {
				continue
			}

			job, err = q.driver.Get(job.ID())
			if job == nil {
				continue
			}

			if err != nil {
				grip.Notice(message.WrapError(err, message.Fields{
					"id":        job.ID(),
					"operation": "problem refreshing job in dispatching from remote queue",
				}))

				grip.Warning(message.WrapError(q.driver.Unlock(job),
					message.Fields{
						"id":        job.ID(),
						"operation": "unlocking job, may leave a stale job",
					}))
				continue
			}

			if !isDispatchable(job.Status()) {
				continue
			}

			ti := amboy.JobTimeInfo{
				Start: time.Now(),
			}
			job.UpdateTimeInfo(ti)

			if err := q.driver.Lock(ctx, job); err != nil {
				grip.Warning(err)
				continue
			}

			grip.Debugf("returning job from remote source, count = %d; duration = %s",
				count, time.Since(start))

			return job
		}
	}
}
