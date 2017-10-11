package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
)

// RemoteUnordered are queues that use a Driver as backend for job
// storage and processing and do not impose any additional ordering
// beyond what's provided by the driver.
type RemoteUnordered struct {
	*remoteBase
}

// NewRemoteUnordered returns a queue that has been initialized with a
// local worker pool Runner instance of the specified size.
func NewRemoteUnordered(size int) *RemoteUnordered {
	q := &RemoteUnordered{
		remoteBase: newRemoteBase(),
	}

	grip.CatchError(q.SetRunner(pool.NewLocalWorkers(size, q)))

	grip.Infof("creating new remote job queue with %d workers", size)

	return q
}

// Next returns a Job from the queue. Returns a nil Job object if the
// context is canceled. The operation is blocking until an
// undispatched, unlocked job is available. This operation takes a job
// lock.
func (q *RemoteUnordered) Next(ctx context.Context) amboy.Job {
	start := time.Now()
	count := 0
	for {
		count++
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			err := q.driver.Lock(job)
			if err != nil {
				grip.Warning(err)
				continue
			}

			job, err = q.driver.Get(job.ID())
			if err != nil {
				grip.CatchNotice(q.driver.Unlock(job))
				grip.Warning(err)
				continue
			}

			grip.Debugf("returning job from remote source, count = %d; duration = %s",
				count, time.Since(start))

			return job
		}
	}
}
