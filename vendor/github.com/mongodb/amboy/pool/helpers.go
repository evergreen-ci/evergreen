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
)

const (
	nilJobWaitIntervalMax = time.Second
	baseJobInterval       = time.Millisecond
)

func jitterNilJobWait() time.Duration {
	return time.Duration(rand.Int63n(int64(nilJobWaitIntervalMax)))

}

func executeJob(ctx context.Context, id string, j amboy.Job, q amboy.Queue) {
	var jobCtx context.Context
	if maxTime := j.TimeInfo().MaxTime; maxTime > 0 {
		var jobCancel context.CancelFunc
		jobCtx, jobCancel = context.WithTimeout(ctx, maxTime)
		defer jobCancel()
	} else {
		jobCtx = ctx
	}
	j.Run(jobCtx)
	if err := q.Complete(ctx, j); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message":  "could not mark job complete",
			"job_id":   j.ID(),
			"queue_id": q.ID(),
		}))
		// If the job cannot not marked as complete in the queue, set the end
		// time so that the calculated job execution statistics are valid.
		j.UpdateTimeInfo(amboy.JobTimeInfo{
			End: time.Now(),
		})
	}

	amboy.WithRetryableQueue(q, func(rq amboy.RetryableQueue) {
		if !j.RetryInfo().ShouldRetry() {
			return
		}

		rh := rq.RetryHandler()
		if rh == nil {
			grip.Error(message.Fields{
				"message":  "cannot retry a job in a queue that does not support retrying",
				"job_id":   j.ID(),
				"queue_id": rq.ID(),
			})
			return
		}

		if err := rh.Put(ctx, j); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "could not prepare job for retry",
				"job_id":   j.ID(),
				"queue_id": rq.ID(),
			}))
		}
	})

	ti := j.TimeInfo()
	msg := message.Fields{
		"job_id":        j.ID(),
		"job_type":      j.Type().Name,
		"duration_secs": ti.Duration().Seconds(),
		"dispatch_secs": ti.Start.Sub(ti.Created).Seconds(),
		"pending_secs":  ti.End.Sub(ti.Created).Seconds(),
		"queue_type":    fmt.Sprintf("%T", q),
		"stat":          j.Status(),
		"pool":          id,
		"max_time_secs": ti.MaxTime.Seconds(),
	}

	if err := j.Error(); err != nil {
		grip.Error(message.WrapError(err, msg))
	} else {
		grip.Info(msg)
	}
}

func worker(bctx context.Context, id string, q amboy.Queue, wg *sync.WaitGroup, mu sync.Locker) {
	var (
		err    error
		job    amboy.Job
		cancel context.CancelFunc
		ctx    context.Context
	)

	mu.Lock()
	wg.Add(1)
	mu.Unlock()

	defer wg.Done()
	defer func() {
		// if we hit a panic we want to add an error to the job;
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
				if err := q.Complete(ctx, job); err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"message":     "could not mark job complete",
						"job_id":      job.ID(),
						"queue_id":    q.ID(),
						"panic_error": err,
					}))
					job.AddError(err)
				}
			}
			// start a replacement worker.
			go worker(bctx, id, q, wg, mu)
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
