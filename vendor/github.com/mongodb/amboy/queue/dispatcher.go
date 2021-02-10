package queue

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Dispatcher provides a common mechanism shared between queue
// implementations to handle job locking to prevent multiple workers
// from running the same job.
type Dispatcher interface {
	Dispatch(context.Context, amboy.Job) error
	Release(context.Context, amboy.Job)
	Complete(context.Context, amboy.Job)
}

type dispatcherImpl struct {
	queue amboy.Queue
	mutex sync.Mutex
	cache map[string]dispatcherInfo
}

// NewDispatcher constructs a default dispatching implementation.
func NewDispatcher(q amboy.Queue) Dispatcher {
	return &dispatcherImpl{
		queue: q,
		cache: map[string]dispatcherInfo{},
	}
}

type dispatcherInfo struct {
	job        amboy.Job
	jobContext context.Context
	jobCancel  context.CancelFunc
	stopPing   context.CancelFunc
}

func (d *dispatcherImpl) Dispatch(ctx context.Context, job amboy.Job) error {
	if job == nil {
		return errors.New("cannot dispatch nil job")
	}

	if !isDispatchable(job.Status(), d.queue.Info().LockTimeout) {
		return errors.New("cannot dispatch in progress or completed job")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if _, ok := d.cache[job.ID()]; ok {
		grip.Debug(message.Fields{
			"message":  "re-dispatching job that has already been dispatched",
			"queue_id": d.queue.ID(),
			"job_id":   job.ID(),
			"service":  "amboy.dispatcher",
		})
		delete(d.cache, job.ID())
	}

	ti := amboy.JobTimeInfo{
		Start: time.Now(),
	}
	job.UpdateTimeInfo(ti)

	info := dispatcherInfo{
		job: job,
	}

	maxTime := job.TimeInfo().MaxTime
	if maxTime > 0 {
		info.jobContext, info.jobCancel = context.WithTimeout(ctx, maxTime)
	} else {
		info.jobContext, info.jobCancel = context.WithCancel(ctx)
	}

	if err := job.Lock(d.queue.ID(), d.queue.Info().LockTimeout); err != nil {
		return errors.Wrap(err, "problem locking job")
	}
	if err := d.queue.Save(ctx, job); err != nil {
		return errors.Wrap(err, "problem saving job state")
	}

	var pingerCtx context.Context
	pingerCtx, info.stopPing = context.WithCancel(ctx)
	go func() {
		defer recovery.LogStackTraceAndContinue("background lock ping", job.ID())
		iters := 0
		ticker := time.NewTicker(d.queue.Info().LockTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-pingerCtx.Done():
				return
			case <-ticker.C:
				if err := job.Lock(d.queue.ID(), d.queue.Info().LockTimeout); err != nil {
					grip.Debug(message.WrapError(err, message.Fields{
						"message":   "failed to lock job for lock ping",
						"queue_id":  d.queue.ID(),
						"job_id":    job.ID(),
						"service":   "amboy.dispatcher",
						"ping_iter": iters,
						"stat":      job.Status(),
					}))
					job.AddError(errors.Wrapf(err, "problem pinging job lock on cycle #%d", iters))
					info.jobCancel()
					return
				}
				if err := d.queue.Save(ctx, job); err != nil {
					grip.DebugWhen(ctx.Err() == nil, message.WrapError(err, message.Fields{
						"message":   "failed to save job for lock ping",
						"queue_id":  d.queue.ID(),
						"job_id":    job.ID(),
						"service":   "amboy.dispatcher",
						"ping_iter": iters,
						"stat":      job.Status(),
					}))
					job.AddError(errors.Wrapf(err, "problem saving job for lock ping on cycle #%d", iters))
					info.jobCancel()
					return
				}
				grip.Debug(message.Fields{
					"queue_id":  d.queue.ID(),
					"job_id":    job.ID(),
					"service":   "amboy.dispatcher",
					"ping_iter": iters,
					"stat":      job.Status(),
				})
			}
			iters++
		}
	}()

	d.cache[job.ID()] = info
	return nil
}

func (d *dispatcherImpl) Release(ctx context.Context, job amboy.Job) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if info, ok := d.cache[job.ID()]; ok {
		info.stopPing()
		info.jobCancel()
		delete(d.cache, job.ID())
	}
}

func (d *dispatcherImpl) Complete(ctx context.Context, job amboy.Job) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	info, ok := d.cache[job.ID()]
	if !ok {
		return
	}
	delete(d.cache, job.ID())

	ti := job.TimeInfo()
	ti.End = time.Now()
	job.UpdateTimeInfo(ti)

	if info.jobContext.Err() != nil && job.Error() == nil {
		job.AddError(errors.New("job was aborted during execution"))
	}

	info.jobCancel()
	info.stopPing()
}
