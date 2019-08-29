package pool

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

const (
	nilJobWaitIntervalMax = time.Second
	baseJobInterval       = time.Millisecond
)

func jitterNilJobWait() time.Duration {
	return time.Duration(rand.Int63n(int64(nilJobWaitIntervalMax)))

}

func executeJob(ctx context.Context, id string, job amboy.Job, q amboy.Queue) {
	res := runJob(ctx, job, q, time.Now())
	ti := job.TimeInfo()
	r := message.Fields{
		"job":           job.ID(),
		"job_type":      job.Type().Name,
		"duration_secs": ti.Duration().Seconds(),
		"queue_type":    fmt.Sprintf("%T", q),
		"stat":          job.Status(),
		"pool":          id,
		"executed":      res.executed,
		"aborted":       res.aborted,
		"max_time_secs": ti.MaxTime.Seconds(),
	}
	err := job.Error()
	if err != nil {
		r["error"] = err.Error()
	}

	if res.executed && !res.aborted && err != nil {
		grip.Error(r)
	} else {
		grip.Debug(r)
	}
}

type runJobResult struct {
	executed bool
	aborted  bool
}

func runJob(ctx context.Context, job amboy.Job, q amboy.Queue, startAt time.Time) (res runJobResult) {
	ti := amboy.JobTimeInfo{
		Start: time.Now(),
	}
	job.UpdateTimeInfo(ti)
	defer func() {
		ti.End = time.Now()
		job.UpdateTimeInfo(ti)
	}()

	maxTime := job.TimeInfo().MaxTime
	if maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, maxTime)
		defer cancel()
	}

	if err := job.Lock(q.ID()); err != nil {
		job.AddError(errors.Wrap(err, "problem locking job"))
		return
	}
	if err := q.Save(ctx, job); err != nil {
		job.AddError(errors.Wrap(err, "problem saving job state"))
		return
	}

	jctx, jcancel := context.WithCancel(ctx)
	defer jcancel()

	pingerCtx, stopPing := context.WithCancel(ctx)
	defer stopPing()
	go func() {
		defer recovery.LogStackTraceAndContinue("background lock ping", job.ID())
		iters := 0
		ticker := time.NewTicker(amboy.LockTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-pingerCtx.Done():
				return
			case <-ticker.C:
				if err := job.Lock(q.ID()); err != nil {
					job.AddError(errors.Wrapf(err, "problem pinging job lock on cycle #%d", iters))
					jcancel()
					return
				}
				if err := q.Save(ctx, job); err != nil {
					job.AddError(errors.Wrapf(err, "problem saving job for lock ping on cycle #%d", iters))
					jcancel()
					return
				}
				grip.Debug(message.Fields{
					"queue_id":  q.ID(),
					"job_id":    job.ID(),
					"ping_iter": iters,
					"stat":      job.Status(),
				})
			}
			iters++
		}
	}()

	job.Run(jctx)
	res.aborted = jctx.Err() != nil
	res.executed = true
	// we want the final end time to include
	// marking complete, but setting it twice is
	// necessary for some queues
	ti.End = time.Now()
	job.UpdateTimeInfo(ti)

	stopPing()

	q.Complete(ctx, job)

	return
}

func worker(bctx context.Context, id string, q amboy.Queue, wg *sync.WaitGroup) {
	var (
		err    error
		job    amboy.Job
		cancel context.CancelFunc
		ctx    context.Context
	)

	wg.Add(1)
	defer wg.Done()
	defer func() {
		// if we hit a panic we want to add an error to the job;
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
				q.Complete(bctx, job)
			}
			// start a replacement worker.
			go worker(bctx, id, q, wg)
		}

		if cancel != nil {
			cancel()
		}
	}()

	timer := time.NewTimer(baseJobInterval)
	defer timer.Stop()
	for {
		select {
		case <-bctx.Done():
			return
		case <-timer.C:
			job := q.Next(bctx)
			if job == nil {
				timer.Reset(jitterNilJobWait())
				continue
			}

			ctx, cancel = context.WithCancel(bctx)
			executeJob(ctx, id, job, q)
			cancel()
			timer.Reset(baseJobInterval)
		}
	}
}
