package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

func runJob(ctx context.Context, job amboy.Job) {
	maxTime := job.TimeInfo().MaxTime
	if maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, maxTime)
		defer cancel()
	}

	job.Run(ctx)
}

func worker(ctx context.Context, jobs <-chan amboy.Job, q amboy.Queue, wg *sync.WaitGroup) {
	var (
		err error
		job amboy.Job
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
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case job = <-jobs:
			if job == nil {
				continue
			}

			ti := amboy.JobTimeInfo{
				Start: time.Now(),
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

			r := message.Fields{
				"job":           job.ID(),
				"job_type":      job.Type().Name,
				"duration_secs": ti.Duration().Seconds(),
				"queue_type":    fmt.Sprintf("%T", q),
			}
			if err := job.Error(); err != nil {
				r["error"] = err.Error()
			}
			grip.Debug(r)
		}
	}
}

func startWorkerServer(ctx context.Context, q amboy.Queue, wg *sync.WaitGroup) <-chan amboy.Job {
	output := make(chan amboy.Job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				job := q.Next(ctx)
				if job == nil {
					continue
				}

				if job.Status().Completed {
					grip.Debugf("job '%s' was dispatched from the queue but was completed",
						job.ID())
					continue
				}

				output <- job
			}
		}
	}()

	return output
}
