package queue

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

////////////////////////////////////////////////////////////////////////
//
// Driver implementation for use with the remote queue implementation
//
////////////////////////////////////////////////////////////////////////

// priorityDriver implements the Driver interface, wrapping a
// PriorityStorage instance. This allows "local" (i.e. intraprocess)
// shared queues that dispatch jobs in priority order.
type priorityDriver struct {
	name    string
	storage *priorityStorage
	closer  context.CancelFunc
	LockManager
}

// NewPriorityDriver returns an initialized Priority Driver instances.
func NewPriorityDriver() Driver {
	p := &priorityDriver{
		name:    uuid.NewV4().String(),
		storage: makePriorityStorage(),
	}

	return p
}

func (p *priorityDriver) ID() string { return p.name }

// Open initilizes the resources of the Driver, and is part of the
// Driver interface. In the case of the Priority Driver, this
// operation cannot error.
func (p *priorityDriver) Open(ctx context.Context) error {
	if p.closer != nil {
		return nil
	}

	_, cancel := context.WithCancel(ctx)
	p.closer = cancel
	p.LockManager = NewLockManager(ctx, p)

	return nil
}

// Close release all resources associated with the Driver instance.
func (p *priorityDriver) Close() {
	if p.closer != nil {
		p.closer()
	}
}

// Get returns a job object, specified by name/ID from the backing
// storage. If the job doesn't exist the error value is non-nil.
func (p *priorityDriver) Get(_ context.Context, name string) (amboy.Job, error) {
	job, ok := p.storage.Get(name)
	if !ok {
		return nil, errors.Errorf("job named '%s' does not exist", name)
	}

	return job, nil
}

// Save updates the stored version of the job in the Driver's backing
// storage. If the job is not tracked by the Driver, this operation is
// an error.
func (p *priorityDriver) Save(_ context.Context, j amboy.Job) error {
	p.storage.Save(j)

	return nil
}

// Put saves a new job returning an error if that job already exists.
func (p *priorityDriver) Put(_ context.Context, j amboy.Job) error {
	return errors.WithStack(p.storage.Insert(j))
}

// SaveStatus persists only the status document in the job in the
// persistence layer. If the job does not exist, this method produces
// an error.
func (p *priorityDriver) SaveStatus(ctx context.Context, j amboy.Job, stat amboy.JobStatusInfo) error {
	job, err := p.Get(ctx, j.ID())
	if err != nil {
		return errors.Wrap(err, "problem saving status")
	}

	stat.ModificationTime = time.Now()
	stat.ModificationCount++

	job.SetStatus(stat)
	if err := p.Save(ctx, job); err != nil {
		return errors.Wrap(err, "problem saving status")
	}

	return nil
}

// Jobs returns an iterator of all Job objects tracked by the Driver.
func (p *priorityDriver) Jobs(_ context.Context) <-chan amboy.Job {
	return p.storage.Contents()
}

// JobStats returns job status documents for all jobs in the storage layer.
func (p *priorityDriver) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	out := make(chan amboy.JobStatusInfo)
	go func() {
		defer close(out)
		for job := range p.storage.Contents() {
			if ctx.Err() != nil {
				return

			}
			status := job.Status()
			status.ID = job.ID()
			out <- status
		}
	}()

	return out
}

// Next returns the next, highest priority Job from the Driver's
// backing storage. If there are no queued jobs, the job object is
// nil.
func (p *priorityDriver) Next(_ context.Context) amboy.Job {
	j := p.storage.Pop()

	if j == nil || j.Status().Completed || j.TimeInfo().IsStale() {
		return nil
	}

	return j

}

// Stats returns a report of the Driver's current state in the form of
// a driver.Stats document.
func (p *priorityDriver) Stats(_ context.Context) amboy.QueueStats {
	stats := amboy.QueueStats{
		Total: p.storage.Size(),
	}

	for job := range p.storage.Contents() {
		stat := job.Status()
		if stat.Completed {
			stats.Completed++
			continue
		}

		if stat.InProgress {
			stats.Running++
			continue
		}

		stats.Pending++
	}

	return stats
}
