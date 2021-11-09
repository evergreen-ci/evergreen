package queue

import (
	"context"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// remoteUnordered implements the amboy.RetryableQueue interface. It uses a
// Driver to access a backend for job storage and processing. The queue does not
// impose any additional job ordering beyond what's provided by the driver.
type remoteUnordered struct {
	*remoteBase
}

// newRemoteUnordered returns a queue that has been initialized with a
// configured local worker pool with the specified number of workers.
func newRemoteUnordered(size int) (remoteQueue, error) {
	return newRemoteUnorderedWithOptions(remoteOptions{numWorkers: size})
}

// newRemoteUnorderedWithOptions returns a queue that has been initialized with
// a configured runner and the given options.
func newRemoteUnorderedWithOptions(opts remoteOptions) (remoteQueue, error) {
	b, err := newRemoteBaseWithOptions(opts)
	if err != nil {
		return nil, errors.Wrap(err, "initializing remote base")
	}
	q := &remoteUnordered{remoteBase: b}
	q.dispatcher = NewDispatcher(q)
	if err := q.SetRunner(pool.NewLocalWorkers(opts.numWorkers, q)); err != nil {
		return nil, errors.Wrap(err, "configuring runner")
	}
	grip.Infof("creating new remote job queue with %d workers", opts.numWorkers)

	return q, nil
}

// Next returns a Job from the queue. Returns a nil Job object if the
// context is canceled. The operation is blocking until an
// undispatched, unlocked job is available. This operation takes a job
// lock.
func (q *remoteUnordered) Next(ctx context.Context) amboy.Job {
	count := 0
	for {
		count++
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			return job
		}
	}
}
