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
	"github.com/pkg/errors"
)

type abortablePool struct {
	queue amboy.Queue
	jobs  map[string]context.CancelFunc

	canceler context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.RWMutex

	size    int
	started bool
}

// NewAbortablePool produces a simple implementation of a worker pool
// that provides access to cancel running jobs. The cancellation
// functions work by creating context cancelation function and then
// canceling the contexts passed to the jobs specifically.
func NewAbortablePool(size int, q amboy.Queue) amboy.AbortableRunner {
	p := &abortablePool{
		queue: q,
		size:  size,
		jobs:  map[string]context.CancelFunc{},
	}

	if p.size <= 0 {
		grip.Infof("setting minimal pool size is 1, overriding setting of '%d'", p.size)
		p.size = 1
	}

	return p
}

func (p *abortablePool) Started() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

func (p *abortablePool) SetQueue(q amboy.Queue) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errors.New("cannot set queue after the pool has started")
	}

	p.queue = q

	return nil
}

func (p *abortablePool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, closer := range p.jobs {
		closer()
		delete(p.jobs, id)
	}

	if p.canceler != nil {
		p.canceler()
		p.canceler = nil
		p.started = false
		p.wg.Wait()
	}

}

func (p *abortablePool) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return nil
	}

	if p.queue == nil {
		return errors.New("runner must have an embedded queue")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	p.canceler = cancel
	jobs := startWorkerServer(workerCtx, p.queue, &p.wg)

	p.started = true

	for w := 1; w <= p.size; w++ {
		go p.worker(workerCtx, jobs)
		grip.Debugf("started worker %d of %d waiting for jobs", w, p.size)
	}

	return nil
}

func (p *abortablePool) worker(ctx context.Context, jobs <-chan workUnit) {
	var (
		err    error
		job    amboy.Job
		cancel context.CancelFunc
	)

	p.mu.Lock()
	p.wg.Add(1)
	p.mu.Unlock()

	defer p.wg.Done()
	defer func() {
		// if we hit a panic we want to add an error to the job;
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
				p.queue.Complete(ctx, job)
			}

			// start a replacement worker.
			go p.worker(ctx, jobs)
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
			p.runJob(ctx, job)
			cancel()
		}
	}
}

func (p *abortablePool) addCanceler(id string, cancel context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.jobs[id] = cancel
}

func (p *abortablePool) runJob(ctx context.Context, job amboy.Job) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	p.addCanceler(job.ID(), cancel)

	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()

		delete(p.jobs, job.ID())
	}()

	start := time.Now()

	executeJob(ctx, job, p.queue, start)

	r := message.Fields{
		"job":           job.ID(),
		"job_type":      job.Type().Name,
		"duration_secs": job.TimeInfo().Duration().Seconds(),
		"queue_type":    fmt.Sprintf("%T", p.queue),
		"pool_type":     "abortable",
	}
	if err := job.Error(); err != nil {
		r["error"] = err.Error()
		grip.Error(r)
	} else {
		grip.Debug(r)
	}
}

func (p *abortablePool) IsRunning(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.jobs[id]

	return ok
}

func (p *abortablePool) RunningJobs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := []string{}

	for id := range p.jobs {
		out = append(out, id)
	}

	return out
}

func (p *abortablePool) Abort(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cancel, ok := p.jobs[id]
	if !ok {
		return errors.Errorf("job '%s' is not defined", id)
	}
	cancel()
	delete(p.jobs, id)

	job, ok := p.queue.Get(id)
	if !ok {
		return errors.Errorf("could not find '%s' in the queue", id)
	}

	p.queue.Complete(ctx, job)

	return nil
}

func (p *abortablePool) AbortAll(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, cancel := range p.jobs {
		if ctx.Err() != nil {
			break
		}
		cancel()
		delete(p.jobs, id)
		job, ok := p.queue.Get(id)
		if !ok {
			continue
		}
		p.queue.Complete(ctx, job)
	}
}
