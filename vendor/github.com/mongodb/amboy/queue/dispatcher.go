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
	// Dispatch allows a single worker to take exclusive ownership of the job
	// when preparing to run it and during its execution. If this succeeds,
	// implementations should not allow any other worker to take ownership of
	// the job unless the job is stranded in progress.
	Dispatch(context.Context, amboy.Job) error
	// Release releases the worker's exclusive ownership of the job.
	Release(context.Context, amboy.Job)
	// Complete relinquishes the worker's exclusive ownership of the job. It may
	// optionally update metadata indicating that the job is finished.
	Complete(context.Context, amboy.Job)
	// Close cleans up all resources used by the Dispatcher and releases all
	// actively-dispatched jobs. Jobs should not be dispatchable after this is
	// called.
	Close(context.Context) error
}

type dispatcherImpl struct {
	queue  amboy.Queue
	mutex  sync.Mutex
	cache  map[string]dispatcherInfo
	closed bool
}

// NewDispatcher constructs a default dispatching implementation.
func NewDispatcher(q amboy.Queue) Dispatcher {
	return &dispatcherImpl{
		queue: q,
		cache: map[string]dispatcherInfo{},
	}
}

type dispatcherInfo struct {
	job           amboy.Job
	pingCtx       context.Context
	pingCancel    context.CancelFunc
	pingCompleted chan struct{}
}

func (d *dispatcherImpl) Dispatch(ctx context.Context, j amboy.Job) error {
	if j == nil {
		return errors.New("cannot dispatch nil job")
	}

	if !isDispatchable(j.Status(), d.queue.Info().LockTimeout) {
		return errors.New("cannot dispatch in progress or completed job")
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed {
		return errors.New("dispatcher is closed")
	}

	if info, ok := d.popInfo(j.ID()); ok {
		grip.Debug(message.Fields{
			"message":  "re-dispatching job that has already been dispatched",
			"queue_id": d.queue.ID(),
			"job_id":   j.ID(),
			"service":  "amboy.queue.dispatcher",
		})
		grip.Warning(message.WrapError(d.waitForPing(ctx, info), message.Fields{
			"message":  "could not wait for job ping to complete",
			"op":       "re-dispatch",
			"job_id":   j.ID(),
			"queue_id": d.queue.ID(),
			"service":  "amboy.queue.dispatcher",
		}))
	}

	ti := amboy.JobTimeInfo{
		Start: time.Now(),
	}
	j.UpdateTimeInfo(ti)

	status := j.Status()
	status.InProgress = true
	j.SetStatus(status)

	info := dispatcherInfo{
		job: j,
	}

	if maxTime := j.TimeInfo().MaxTime; maxTime > 0 {
		info.pingCtx, info.pingCancel = context.WithTimeout(ctx, maxTime)
	} else {
		info.pingCtx, info.pingCancel = context.WithCancel(ctx)
	}

	if err := j.Lock(d.queue.ID(), d.queue.Info().LockTimeout); err != nil {
		return errors.Wrap(err, "problem locking job")
	}
	if err := d.queue.Save(ctx, j); err != nil {
		return errors.Wrap(err, "problem saving job state")
	}

	info.pingCompleted = make(chan struct{})
	go func() {
		defer func() {
			if err := recovery.HandlePanicWithError(recover(), nil, "background lock ping"); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"job_id":   j.ID(),
					"service":  "amboy.queue.dispatcher",
					"queue_id": d.queue.ID(),
				}))
				j.AddError(err)
			}

			close(info.pingCompleted)
		}()

		if err := pingJobLock(info.pingCtx, d.queue, j); err != nil {
			grip.WarningWhen(info.pingCtx.Err() == nil, message.WrapError(err, message.Fields{
				"message":  "could not ping job lock",
				"job_id":   j.ID(),
				"service":  "amboy.queue.dispatcher",
				"queue_id": d.queue.ID(),
			}))
			j.AddError(err)
		}
	}()

	d.cache[j.ID()] = info

	return nil
}

func (d *dispatcherImpl) Release(ctx context.Context, j amboy.Job) {
	d.mutex.Lock()
	info, ok := d.popInfo(j.ID())
	if !ok {
		d.mutex.Unlock()
		return
	}
	d.mutex.Unlock()

	grip.Warning(message.WrapError(d.waitForPing(ctx, info), message.Fields{
		"message":  "could not wait for job ping to complete",
		"op":       "release",
		"job_id":   j.ID(),
		"queue_id": d.queue.ID(),
		"service":  "amboy.queue.dispatcher",
	}))
}

// popInfo retrieves and removes information related to the dispatch of a job,
// returning the removed information to the caller and whether or not the
// information was found.
func (d *dispatcherImpl) popInfo(jobID string) (dispatcherInfo, bool) {
	info, ok := d.cache[jobID]
	delete(d.cache, jobID)
	return info, ok
}

func (d *dispatcherImpl) Complete(ctx context.Context, j amboy.Job) {
	d.mutex.Lock()
	info, ok := d.popInfo(j.ID())
	if !ok {
		d.mutex.Unlock()
		return
	}
	d.mutex.Unlock()

	ti := j.TimeInfo()
	ti.End = time.Now()
	j.UpdateTimeInfo(ti)

	if info.pingCtx.Err() != nil && j.Error() == nil {
		j.AddError(errors.New("job was aborted during execution"))
	}

	grip.Warning(message.WrapError(d.waitForPing(ctx, info), message.Fields{
		"message":  "could not wait for job ping to complete",
		"op":       "complete",
		"job_id":   j.ID(),
		"queue_id": d.queue.ID(),
		"service":  "amboy.queue.dispatcher",
	}))
}

// Close releases all jobs currently locked by the dispatcher and waits for
// all ping threads to exit.
func (d *dispatcherImpl) Close(ctx context.Context) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.closed {
		return nil
	}

	d.closed = true

	catcher := grip.NewBasicCatcher()
	for jobID := range d.cache {
		info, ok := d.popInfo(jobID)
		if !ok {
			continue
		}
		catcher.Wrapf(d.waitForPing(ctx, info), "waiting for job ping to complete for job '%s'", jobID)
	}

	return catcher.Resolve()
}

// waitForPing cancels the dispatcher ping for a job and waits for the pinger to
// exit.
func (d *dispatcherImpl) waitForPing(ctx context.Context, info dispatcherInfo) error {
	info.pingCancel()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-info.pingCompleted:
		return nil
	}
}

func pingJobLock(ctx context.Context, q amboy.Queue, j amboy.Job) error {
	var iters int
	lockTimeout := q.Info().LockTimeout
	ticker := time.NewTicker(lockTimeout / 4)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			status := j.Status()
			status.InProgress = true
			status.Completed = false
			j.SetStatus(status)

			if err := j.Lock(q.ID(), q.Info().LockTimeout); err != nil {
				return errors.Wrapf(err, "pinging job lock on cycle #%d", iters)
			}
			if err := q.Save(ctx, j); err != nil {
				return errors.Wrapf(err, "saving job for lock ping on cycle #%d", iters)
			}

			grip.Debug(message.Fields{
				"queue_id":  q.ID(),
				"job_id":    j.ID(),
				"service":   "amboy.queue.dispatcher",
				"ping_iter": iters,
				"stat":      j.Status(),
			})

			iters++
		}
	}
}
