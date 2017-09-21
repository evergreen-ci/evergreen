package driver

import (
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/net/context"
)

////////////////////////////////////////////////////////////////////////
//
// Driver implementation for use with the remote queue implementation
//
////////////////////////////////////////////////////////////////////////

// Priority implements the Driver interface, wrapping a
// PriorityStorage instance. This allows "local" (i.e. intraprocess)
// shared queues that dispatch jobs in priority order.
type Priority struct {
	storage *PriorityStorage
	closer  context.CancelFunc
	*LockManager
}

// NewPriority returns an initialized Priority Driver instances.
func NewPriority() *Priority {
	p := &Priority{
		storage: NewPriorityStorage(),
	}
	p.LockManager = NewLockManager(uuid.NewV4().String(), p)

	return p
}

// Open initilizes the resources of the Driver, and is part of the
// Driver interface. In the case of the Priority Driver, this
// operation cannot error.
func (p *Priority) Open(ctx context.Context) error {
	if p.closer != nil {
		return nil
	}

	_, cancel := context.WithCancel(ctx)
	p.closer = cancel
	p.LockManager.Open(ctx)

	return nil
}

// Close release all resources associated with the Driver instance.
func (p *Priority) Close() {
	if p.closer != nil {
		p.closer()
	}
}

// Get returns a job object, specified by name/ID from the backing
// storage. If the job doesn't exist the error value is non-nil.
func (p *Priority) Get(name string) (amboy.Job, error) {
	job, ok := p.storage.Get(name)
	if !ok {
		return nil, errors.Errorf("job named '%s' does not exist", name)
	}

	return job, nil
}

// Save updates the stored version of the job in the Driver's backing
// storage. If the job is not tracked by the Driver, this operation is
// an error.
func (p *Priority) Save(j amboy.Job) error {
	p.storage.Push(j)

	return nil
}

// SaveStatus persists only the status document in the job in the
// persistence layer. If the job does not exist, this method produces
// an error.
func (p *Priority) SaveStatus(j amboy.Job, stat amboy.JobStatusInfo) error {
	job, err := p.Get(j.ID())
	if err != nil {
		return errors.Wrap(err, "problem saving status")
	}

	job.SetStatus(stat)
	if err := p.Save(job); err != nil {
		return errors.Wrap(err, "problem saving status")
	}

	return nil
}

// Jobs returns an iterator of all Job objects tracked by the Driver.
func (p *Priority) Jobs() <-chan amboy.Job {
	return p.storage.Contents()
}

// Next returns the next, highest priority Job from the Driver's
// backing storage. If there are no queued jobs, the job object is
// nil.
func (p *Priority) Next() amboy.Job {
	j := p.storage.Pop()

	if j == nil || j.Status().Completed {
		return nil
	}

	return j

}

// Stats returns a report of the Driver's current state in the form of
// a driver.Stats document.
func (p *Priority) Stats() amboy.QueueStats {
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
