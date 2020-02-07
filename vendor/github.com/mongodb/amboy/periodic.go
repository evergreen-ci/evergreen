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
type QueueOperation func(context.Context, Queue) error

// QueueOperationConfig describes the behavior of the periodic
// interval schedulers.
//
// The theshold, if ResepectThreshold is set, causes the periodic
// scheduler to noop if there are more than that many pending jobs.
type QueueOperationConfig struct {
	ContinueOnError  bool `bson:"continue_on_error" json:"continue_on_error" yaml:"continue_on_error"`
	LogErrors        bool `bson:"log_errors" json:"log_errors" yaml:"log_errors"`
	DebugLogging     bool `bson:"debug_logging" json:"debug_logging" yaml:"debug_logging"`
	RespectThreshold bool `bson:"respect_threshold" json:"respect_threshold" yaml:"respect_threshold"`
	Threshold        int  `bson:"threshold" json:"threshold" yaml:"threshold"`
}

// ScheduleJobFactory produces a QueueOpertion that calls a single
// function which returns a Job and puts that job into the queue.
func ScheduleJobFactory(op func() Job) QueueOperation {
	return func(ctx context.Context, q Queue) error {
		return q.Put(ctx, op())
	}
}

// ScheduleManyJobsFactory produces a queue operation that calls a
// single function which returns a slice of jobs and puts those jobs into
// the queue. The QueueOperation attempts to add all jobs in the slice
// and returns an error if the Queue.Put opertion failed for any
// (e.g. continue-on-error semantics). The error returned aggregates
// all errors encountered.
func ScheduleManyJobsFactory(op func() []Job) QueueOperation {
	return func(ctx context.Context, q Queue) error {
		catcher := grip.NewCatcher()
		for _, j := range op() {
			catcher.Add(q.Put(ctx, j))
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
	return func(ctx context.Context, q Queue) error {
		catcher := grip.NewCatcher()

		jobs := op()
	waitLoop:
		for {
			select {
			case j := <-jobs:
				catcher.Add(q.Put(ctx, j))
			case <-ctx.Done():
				catcher.Add(ctx.Err())
				break waitLoop
			}
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
	return func(ctx context.Context, q Queue) error {
		catcher := grip.NewCatcher()

		catcher.Add(first(ctx, q))

		for _, op := range ops {
			if err := ctx.Err(); err != nil {
				catcher.Add(err)
				break
			}
			catcher.Add(op(ctx, q))
		}
		return catcher.Resolve()
	}
}

// IntervalQueueOperation runs a queue scheduling operation on a
// regular interval, starting at specific time. Use this method to
// schedule jobs every hour, or similar use-cases.
func IntervalQueueOperation(ctx context.Context, q Queue, interval time.Duration, startAt time.Time, conf QueueOperationConfig, op QueueOperation) {
	go func() {
		var err error

		if interval <= time.Microsecond {
			grip.Criticalf("invalid interval queue operation '%s'", interval)
			return
		}

		defer func() {
			err = recovery.HandlePanicWithError(recover(), err, "interval background job scheduler")

			if err != nil {
				if !conf.ContinueOnError {
					return
				}

				if ctx.Err() != nil {
					return
				}

				IntervalQueueOperation(ctx, q, interval, startAt, conf, op)
			}
		}()

		waitUntilInterval(ctx, startAt, interval)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		if ctx.Err() != nil {
			return
		}

		count := 1
		if err = scheduleOp(ctx, q, op, conf); err != nil {
			return
		}

		for {
			select {
			case <-ctx.Done():
				grip.InfoWhen(conf.DebugLogging, message.Fields{
					"message":       "exiting interval job scheduler",
					"num_intervals": count,
					"queue":         "single",
					"reason":        "operation canceled",
					"conf":          conf,
				})
				return
			case <-ticker.C:
				if err = scheduleOp(ctx, q, op, conf); err != nil {
					return
				}

				count++
			}
		}
	}()
}

func scheduleOp(ctx context.Context, q Queue, op QueueOperation, conf QueueOperationConfig) error {
	if conf.RespectThreshold && q.Stats(ctx).Pending > conf.Threshold {
		return nil
	}

	if err := errors.Wrap(op(ctx, q), "problem encountered during periodic job scheduling"); err != nil {
		if conf.ContinueOnError {
			grip.WarningWhen(conf.LogErrors, err)
		} else {
			grip.CriticalWhen(conf.LogErrors, err)
			return err
		}
	}

	return nil
}

func waitUntilInterval(ctx context.Context, startAt time.Time, interval time.Duration) {
	if startAt.Before(time.Now()) {
		for {
			startAt = startAt.Add(interval)
			if startAt.Before(time.Now()) {
				continue
			}

			break
		}
	}

	timer := time.NewTimer(-time.Since(startAt))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}
