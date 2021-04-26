/*
Waiting for Jobs to Complete

The amboy package proves a number of generic methods that, using the
Queue.Stats() method, block until all jobs are complete. They provide different
semantics, which may be useful in different circumstances. All of the Wait*
functions wait until the total number of jobs submitted to the queue is equal to
the number of completed jobs, and as a result these methods don't prevent other
threads from adding jobs to the queue after beginning to wait. As a special
case, retryable queues will also wait until there are no retrying jobs
remaining.

Additionally, there are a set of methods, WaitJob*, that allow callers to wait
for a specific job to complete.
*/
package amboy

import (
	"context"
	"time"
)

// Wait takes a queue and blocks until all job are completed or the context is
// canceled. This operation runs in a tight-loop, which means that the Wait will
// return *as soon* as all jobs are complete. Conversely, it's also possible
// that frequent repeated calls to Stats() may contend with resources needed for
// dispatching jobs or marking them complete. Retrying jobs are not considered
// complete.
func Wait(ctx context.Context, q Queue) bool {
	for {
		if ctx.Err() != nil {
			return false
		}

		stat := q.Stats(ctx)
		if stat.IsComplete() {
			return true
		}

	}
}

// WaitInterval provides the Wait operation and accepts a context for
// cancellation while also waiting for an interval between stats calls. The
// return value reports if the operation was canceled or if all jobs are
// complete. Retrying jobs are not considered complete.
func WaitInterval(ctx context.Context, q Queue, interval time.Duration) bool {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			if q.Stats(ctx).IsComplete() {
				return true
			}

			timer.Reset(interval)
		}
	}
}

// WaitIntervalNum waits for a certain number of jobs to complete. Retrying jobs
// are not considered complete.
func WaitIntervalNum(ctx context.Context, q Queue, interval time.Duration, num int) bool {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			if q.Stats(ctx).Completed-q.Stats(ctx).Retrying >= num {
				return true
			}
		}
	}
}

// WaitJob blocks until the job, based on its ID, is marked complete in the
// queue, or the context is canceled. The return value is false if the job does
// not exist (or is removed) and true when the job completes. A retrying job is
// not considered complete.
func WaitJob(ctx context.Context, j Job, q Queue) bool {
	var ok bool
	for {
		if ctx.Err() != nil {
			return false
		}

		j, ok = q.Get(ctx, j.ID())
		if !ok {
			return false
		}

		if ctx.Err() != nil {
			return false
		}

		completed := j.Status().Completed && j.RetryInfo().ShouldRetry()
		if completed {
			return true
		}
	}
}

// WaitJobInterval takes a job and queue object and waits for the job to be
// marked complete. The interval parameter controls how long the operation waits
// between checks, and can be used to limit the impact of waiting on a busy
// queue. The operation returns false if the job is not registered in the queue,
// and true when the job completes. A retrying job is not considered complete.
func WaitJobInterval(ctx context.Context, j Job, q Queue, interval time.Duration) bool {
	var ok bool

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			j, ok = q.Get(ctx, j.ID())
			if !ok {
				return false
			}

			completed := j.Status().Completed && !j.RetryInfo().ShouldRetry()

			if completed {
				return true
			}

			timer.Reset(interval)
		}
	}
}
