/*
Waiting for Jobs to Complete

The amboy package proves a number of generic methods that, using the
Queue.Stats() method, block until all jobs are complete. They provide
different semantics, which may be useful in different
circumstances. All of these functions wait until the total number of
jobs submitted to the queue is equal to the number of completed jobs,
and as a result these methods don't prevent other threads from adding
jobs to the queue after beginning to wait.

Additionally, there are a set of methods that allow callers to wait for
a specific job to complete.
*/
package amboy

import (
	"context"
	"time"
)

// Wait takes a queue and blocks until all tasks are completed. This
// operation runs in a tight-loop, which means that the Wait will
// return *as soon* as possible all tasks or complete. Conversely,
// it's also possible that frequent repeated calls to Stats() may
// contend with resources needed for dispatching jobs or marking them
// complete.
func Wait(q Queue) {
	for {
		if q.Stats().isComplete() {
			break
		}
	}
}

// WaitCtx make it possible to cancel, either directly or using a
// deadline or timeout, a Wait operation using a context object. The
// return value is true if all tasks are complete, and false if the
// operation returns early because it was canceled.
func WaitCtx(ctx context.Context, q Queue) bool {
	for {
		if ctx.Err() != nil {
			return false
		}

		stat := q.Stats()
		if stat.isComplete() {
			return true
		}

	}
}

// WaitInterval adds a sleep between stats calls, as a way of
// throttling the impact of repeated Stats calls to the queue.
func WaitInterval(q Queue, interval time.Duration) {
	for {
		if q.Stats().isComplete() {
			break
		}

		time.Sleep(interval)
	}
}

// WaitCtxInterval provides the Wait operation and accepts a context
// for cancellation while also waiting for an interval between stats
// calls. The return value reports if the operation was canceled or if
// all tasks are complete.
func WaitCtxInterval(ctx context.Context, q Queue, interval time.Duration) bool {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			if q.Stats().isComplete() {
				return true
			}

			timer.Reset(interval)
		}
	}
}

// WaitJob blocks until the job, based on its ID, is marked complete
// in the queue. The return value is false if the job does not exist
// (or is removed) and true when the job completes. This operation could
// block indefinitely.
func WaitJob(j Job, q Queue) bool {
	var ok bool

	for {
		j, ok = q.Get(j.ID())
		if !ok {
			return false
		}

		if j.Status().Completed {
			return true
		}
	}
}

// WaitJobCtx blocks until the job, based on its ID, is marked complete
// in the queue. This operation blocks indefinitely, unless the
// context is canceled or reaches its timeout. The return value is
// false if the job does not exist or if the context is canceled, and
// only returns true when the job is complete.
func WaitJobCtx(ctx context.Context, j Job, q Queue) bool {
	var ok bool
	for {
		if ctx.Err() != nil {
			return false
		}

		j, ok = q.Get(j.ID())
		if !ok {
			return false
		}

		if ctx.Err() != nil {
			return false
		}

		if j.Status().Completed {
			return true
		}
	}
}

// WaitJobInterval takes a job and queue object and waits for the job
// to be marked complete. The interval parameter controls how long the
// operation waits between checks, and can be used to limit the impact
// of waiting on a busy queue. The operation returns false if the job
// is not registered in the queue, and true when the job completes.
func WaitJobInterval(j Job, q Queue, interval time.Duration) bool {
	var ok bool

	for {
		j, ok = q.Get(j.ID())
		if !ok {
			return false
		}

		if j.Status().Completed {
			return true
		}

		time.Sleep(interval)
	}
}

// WaitJobCtxInterval waits for a job in a queue to complete. Returns
// false if the context has been canceled, or if the job does not exist
// in the queue, and true only after the job is marked complete.
func WaitJobCtxInterval(ctx context.Context, j Job, q Queue, interval time.Duration) bool {
	var ok bool

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			j, ok = q.Get(j.ID())
			if !ok {
				return false
			}

			if j.Status().Completed {
				return true
			}

			timer.Reset(interval)
		}
	}
}
