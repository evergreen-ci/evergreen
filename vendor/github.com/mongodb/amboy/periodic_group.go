package amboy

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// GroupQueueOperation describes a single queue population operation
// for a group queue.
type GroupQueueOperation struct {
	Operation QueueOperation
	Queue     string
	Check     func(context.Context) bool
}

// IntervalGroupQueueOperation schedules jobs on a queue group with
// similar semantics as IntervalQueueOperation.
//
// Operations will continue to run as long as the context is not
// canceled. If you do not pass any GroupQueueOperation items to this
// function, it panics.
func IntervalGroupQueueOperation(ctx context.Context, qg QueueGroup, interval time.Duration, startAt time.Time, conf QueueOperationConfig, ops ...GroupQueueOperation) {
	if len(ops) == 0 {
		panic("queue group operation must contain operations")
	}

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

				IntervalGroupQueueOperation(ctx, qg, interval, startAt, conf, ops...)
			}
		}()

		waitUntilInterval(ctx, startAt, interval)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		if ctx.Err() != nil {
			return
		}
		count := 1
		for _, op := range ops {
			if op.Check == nil || op.Check(ctx) {
				if err = scheduleGroupOp(ctx, qg, op, conf); err != nil {
					return
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				grip.InfoWhen(conf.DebugLogging, message.Fields{
					"message":       "exiting interval job scheduler",
					"queue":         "group",
					"num_intervals": count,
					"reason":        "operation canceled",
					"conf":          conf,
				})
				return
			case <-ticker.C:
				for _, op := range ops {
					if err := scheduleGroupOp(ctx, qg, op, conf); err != nil {
						return
					}
				}

				count++
			}
		}
	}()
}

func scheduleGroupOp(ctx context.Context, group QueueGroup, op GroupQueueOperation, conf QueueOperationConfig) error {
	if op.Check == nil || op.Check(ctx) {
		q, err := group.Get(ctx, op.Queue)
		if conf.ContinueOnError {
			grip.WarningWhen(conf.LogErrors, err)
		} else {
			grip.CriticalWhen(conf.LogErrors, err)
			return errors.Wrapf(err, "problem getting queue '%s' from group", op.Queue)
		}

		if err = scheduleOp(ctx, q, op.Operation, conf); err != nil {
			return errors.Wrapf(err, "problem scheduling job on group queue '%s'", op.Queue)
		}
	}
	return nil
}
