package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const dispatchWarningThreshold = time.Second

// RemoteUnordered are queues that use a Driver as backend for job
// storage and processing and do not impose any additional ordering
// beyond what's provided by the driver.
type remoteUnordered struct {
	*remoteBase
}

// newRemoteUnordered returns a queue that has been initialized with a
// local worker pool Runner instance of the specified size.
func newRemoteUnordered(size int) remoteQueue {
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
	getErrors := 0
	dispatchableErrors := 0
	for {
		count++
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			if job == nil {
				continue
			}

			job, err = q.driver.Get(ctx, job.ID())
			if job == nil {
				getErrors++
				continue
			}

			if err != nil {
				grip.Debug(message.WrapError(err, message.Fields{
					"id":        job.ID(),
					"operation": "problem refreshing job in dispatching from remote queue",
				}))

				getErrors++
				continue
			}

			status := job.Status()
			if !isDispatchable(status) {
				dispatchableErrors++
				continue
			}

			ti := amboy.JobTimeInfo{
				Start: time.Now(),
			}
			job.UpdateTimeInfo(ti)

			dispatchSecs := time.Since(start).Seconds()
			grip.DebugWhen(dispatchSecs > dispatchWarningThreshold.Seconds() || count > 3,
				message.Fields{
					"message":             "returning job from remote source",
					"threshold_secs":      dispatchWarningThreshold.Seconds(),
					"dispatch_secs":       dispatchSecs,
					"attempts":            count,
					"stat":                status,
					"job":                 job.ID(),
					"get_errors":          getErrors,
					"dispatchable_errors": dispatchableErrors,
				})

			return job
		}
	}
}
