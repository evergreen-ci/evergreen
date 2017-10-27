package amboy

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// QueueOperation is a named function literal for use in the
// PeriodicQueueOperation function. Typically these functions add jobs
// to a queue, or could be used to perform periodic maintenance
// (e.g. removing stale jobs or removing stuck jobs in a dependency
// queue.)
type QueueOperation func(Queue) error

// ScheduleJobFactory produces a QueueOpertion that calls a single
// function which returns a Job and puts that job into the queue.
func ScheduleJobFactory(op func() Job) QueueOperation {
	return func(q Queue) error {
		return q.Put(op())
	}
}

// ScheduleManyJobsFactory produces a queue operation that calls a
// single function which returns a slice of jobs and puts those jobs into
// the queue. The QueueOperation attempts to add all jobs in the slice
// and returns an error if the Queue.Put opertion failed for any
// (e.g. continue-on-error semantics). The error returned aggregates
// all errors encountered.
func ScheduleManyJobsFactory(op func() []Job) QueueOperation {
	return func(q Queue) error {
		catcher := grip.NewCatcher()
		for _, j := range op() {
			catcher.Add(q.Put(j))
		}
		return catcher.Resolve()
	}
}

// ScheduleJobsFromGeneratorFactory produces a queue operation that calls a
// single generator function which returns channel of Jobs and puts those
// jobs into the queue. The QueueOperation attempts to add all jobs in
// the slice and returns an error if the Queue.Put opertion failed for
// any (e.g. continue-on-error semantics). The error returned aggregates
// all errors encountered.
func ScheduleJobsFromGeneratorFactory(op func() <-chan Job) QueueOperation {
	return func(q Queue) error {
		catcher := grip.NewCatcher()
		for j := range op() {
			catcher.Add(q.Put(j))
		}
		return catcher.Resolve()
	}
}

// GroupQueueOperationFactory produces a QueueOperation that
// aggregates and runs one or more QueueOperations. The QueueOperation
// has continue-on-error semantics, and returns an error if any of the
// QueueOperations fail, but attempts to run all specified
// QueueOperations before propagating errors.
func GroupQueueOperationFactory(first QueueOperation, ops ...QueueOperation) QueueOperation {
	return func(q Queue) error {
		catcher := grip.NewCatcher()

		catcher.Add(first(q))

		for _, op := range ops {
			catcher.Add(op(q))
		}

		return catcher.Resolve()
	}
}

// PeriodicQueueOperation launches a goroutine that runs the
// QueueOperation on the specified Queue at the specified interval. If
// ignoreErrors is true, then a QueueOperation that returns an error will
// *not* interrupt the background process. Otherwise, the background
// process will exit if a QueueOperation fails. Use the context to
// terminate the background process.
func PeriodicQueueOperation(ctx context.Context, q Queue, interval time.Duration, ignoreErrors bool, op QueueOperation) {
	go func() {
		var err error

		defer func() {
			err = recovery.HandlePanicWithError(recover(), err, "periodic background scheduler error")
			if err != nil {
				if !ignoreErrors {
					return
				}

				if ctx.Err() != nil {
					return
				}

				PeriodicQueueOperation(ctx, q, interval, ignoreErrors, op)
			}
		}()

		timer := time.NewTimer(0)
		defer timer.Stop()
		count := 0

		for {
			select {
			case <-ctx.Done():
				grip.Info(message.Fields{
					"message":    "exiting periodic job scheduler",
					"numPeriods": count,
				})
				return
			case <-timer.C:
				if err = scheduleOp(q, op, ignoreErrors); err != nil {
					return
				}

				count++
				timer.Reset(interval)
			}
		}
	}()
}

// IntervalQueueOperation runs a queue scheduling operation on a
// regular interval, starting at specific time. Use this method to
// schedule jobs every hour, or similar use-cases.
func IntervalQueueOperation(ctx context.Context, q Queue, interval time.Duration, startAt time.Time, ignoreErrors bool, op QueueOperation) {
	go func() {
		var err error

		defer func() {
			err = recovery.HandlePanicWithError(recover(), err, "interval background job scheduler")

			if err != nil {
				if !ignoreErrors {
					return
				}

				if ctx.Err() != nil {
					return
				}

				IntervalQueueOperation(ctx, q, interval, startAt, ignoreErrors, op)
			}
		}()

		initialWait := time.Since(startAt)
		if initialWait > time.Second {
			grip.Infof("waiting %s to start scheduling an interval job", initialWait)
			time.Sleep(initialWait)
		}

		if err = scheduleOp(q, op, ignoreErrors); err != nil {
			return
		}

		count := 0
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				grip.Info(message.Fields{
					"message":      "exiting interval job scheduler",
					"numIntervals": count,
				})
				return
			case <-ticker.C:
				if err = scheduleOp(q, op, ignoreErrors); err != nil {
					return
				}

				count++
			}
		}
	}()
}

func scheduleOp(q Queue, op QueueOperation, ignoreErrors bool) error {
	if err := errors.Wrap(op(q), "problem encountered during periodic job scheduling"); err != nil {
		if ignoreErrors {
			grip.Warning(err)
		} else {
			grip.Critical(err)
			return err
		}
	}

	return nil
}
