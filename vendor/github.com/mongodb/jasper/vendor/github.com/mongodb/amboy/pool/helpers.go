package pool

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
)

type workUnit struct {
	job    amboy.Job
	cancel context.CancelFunc
}

func executeJob(ctx context.Context, job amboy.Job, q amboy.Queue, startAt time.Time) {
	ti := amboy.JobTimeInfo{
		Start: startAt,
	}
	job.UpdateTimeInfo(ti)

	runJob(ctx, job)

	// we want the final end time to include
	// marking complete, but setting it twice is
	// necessary for some queues
	ti.End = time.Now()
	job.UpdateTimeInfo(ti)

	q.Complete(ctx, job)
	ti.End = time.Now()
	job.UpdateTimeInfo(ti)
}

func runJob(ctx context.Context, job amboy.Job) {
	maxTime := job.TimeInfo().MaxTime
	if maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, maxTime)
		defer cancel()
	}

	job.Run(ctx)
}

func worker(ctx context.Context, jobs <-chan workUnit, q amboy.Queue, wg *sync.WaitGroup) {
	var (
		err    error
		job    amboy.Job
		cancel context.CancelFunc
	)

	wg.Add(1)
	defer wg.Done()
	defer func() {
		// if we hit a panic we want to add an error to the job;
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
				q.Complete(ctx, job)
			}
			// start a replacement worker.
			go worker(ctx, jobs, q, wg)
		}

		if cancel != nil {
			cancel()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case wu := <-jobs:
			if wu.job == nil {
				continue
			}

			job = wu.job
			cancel = wu.cancel
			executeJob(ctx, job, q, time.Now())
			cancel()
		}
	}
}

func startWorkerServer(ctx context.Context, q amboy.Queue, wg *sync.WaitGroup) <-chan workUnit {
	var nctx context.Context

	output := make(chan workUnit)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				wu := workUnit{}
				nctx, wu.cancel = context.WithCancel(ctx)

				job := q.Next(nctx)
				if job == nil {
					continue
				}

				if job.Status().Completed {
					grip.Debugf("job '%s' was dispatched from the queue but was completed",
						job.ID())
					continue
				}
				wu.job = job
				output <- wu
			}
		}
	}()

	return output
}
