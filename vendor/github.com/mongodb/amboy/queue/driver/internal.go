package driver

import (
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/net/context"
)

// Internal implements the driver interface, but rather than
// connecting to a remote data source, this implementation is mostly
// for testing the queue implementation locally, and providing a proof
// of concept for the remote driver. May also be useful for converting
// a remote queue into a local-only architecture in a
// dependency-injection situation.
type Internal struct {
	jobs struct {
		dispatched map[string]struct{}
		pending    []string
		m          map[string]amboy.Job
		sync.RWMutex
	}
	closer context.CancelFunc
	*LockManager
}

// NewInternal creates a local persistence layer object.
func NewInternal() *Internal {
	d := &Internal{}
	d.jobs.m = make(map[string]amboy.Job)
	d.jobs.dispatched = make(map[string]struct{})
	d.LockManager = NewLockManager(uuid.NewV4().String(), d)
	return d
}

// Open is a noop for the Internal implementation, and exists to
// satisfy the Driver interface.
func (d *Internal) Open(ctx context.Context) error {
	_, cancel := context.WithCancel(ctx)
	d.closer = cancel
	d.LockManager.Open(ctx)

	return nil
}

// Close is a noop for the Internal implementation, and exists to
// satisfy the Driver interface.
func (d *Internal) Close() {
	if d.closer != nil {
		d.closer()
	}
}

// Get retrieves a job object from the persistence system based on the
// name (ID) of the job. If no job exists by this name, the error is
// non-nil.
func (d *Internal) Get(name string) (amboy.Job, error) {
	d.jobs.RLock()
	defer d.jobs.RUnlock()

	j, ok := d.jobs.m[name]

	if ok {
		return j, nil
	}

	return nil, errors.Errorf("no job named %s exists", name)
}

// Save takes a job and persists it in the storage for this driver. If
// there is no job with a matching ID, then this operation returns an
// error.
func (d *Internal) Save(j amboy.Job) error {
	d.jobs.Lock()
	defer d.jobs.Unlock()
	name := j.ID()

	if j.Status().Completed {
		delete(d.jobs.dispatched, name)
	} else if _, ok := d.jobs.m[name]; !ok {
		d.jobs.pending = append(d.jobs.pending, name)
	}

	d.jobs.m[name] = j

	grip.Debugf("saving job %s", name)
	return nil
}

// SaveStatus persists only the status document in the job in the
// persistence layer. If the job does not exist, this method produces
// an error.
func (d *Internal) SaveStatus(j amboy.Job, stat amboy.JobStatusInfo) error {
	d.jobs.Lock()
	defer d.jobs.Unlock()
	name := j.ID()

	if job, ok := d.jobs.m[name]; ok {
		job.SetStatus(stat)
		d.jobs.m[name] = job
		return nil
	}

	return errors.Errorf("cannot save a status for job named %s, which doesn't exist.", name)
}

// Jobs is a generator of all Job objects stored by the driver. There
// is no additional filtering of the jobs produced by this generator.
func (d *Internal) Jobs() <-chan amboy.Job {
	d.jobs.RLock()
	defer d.jobs.RUnlock()
	output := make(chan amboy.Job, len(d.jobs.m))

	go func() {
		d.jobs.RLock()
		defer d.jobs.RUnlock()
		for _, job := range d.jobs.m {
			output <- job
		}
		close(output)
	}()

	return output
}

// Next returns a job that is not complete from the queue. If there
// are no pending jobs, then this method returns nil, but does not
// block.
func (d *Internal) Next() amboy.Job {
	d.jobs.Lock()
	defer d.jobs.Unlock()

	if len(d.jobs.pending) == 0 {
		return nil
	}

	for idx, name := range d.jobs.pending {
		// delete item from pending slice at index
		d.jobs.pending[idx] = d.jobs.pending[len(d.jobs.pending)-1]
		d.jobs.pending[len(d.jobs.pending)-1] = ""
		d.jobs.pending = d.jobs.pending[:len(d.jobs.pending)-1]

		if _, ok := d.jobs.dispatched[name]; ok {
			continue
		}

		job := d.jobs.m[name]
		if job.Status().Completed {
			d.jobs.dispatched[name] = struct{}{}
			continue
		}

		d.jobs.dispatched[name] = struct{}{}
		return job
	}

	// if we get here then there are no pending jobs and we should
	// just return nil
	return nil
}

// Stats iterates through all of the jobs stored in the driver and
// determines how many locked, completed, and pending jobs are stored
// in the queue.
func (d *Internal) Stats() amboy.QueueStats {
	d.jobs.RLock()
	defer d.jobs.RUnlock()

	stats := amboy.QueueStats{
		Total: len(d.jobs.m),
	}

	for _, j := range d.jobs.m {
		stat := j.Status()
		if stat.Completed {
			stats.Completed++
			continue
		}

		if stat.InProgress && stat.Owner != "" {
			stats.Running++
			continue
		}
		stats.Pending++
	}

	return stats
}
