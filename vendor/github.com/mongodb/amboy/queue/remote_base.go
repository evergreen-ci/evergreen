package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type remoteQueue interface {
	amboy.Queue
	SetDriver(remoteQueueDriver) error
	Driver() remoteQueueDriver
}

type remoteBase struct {
	started    bool
	driver     remoteQueueDriver
	driverType string
	channel    chan amboy.Job
	blocked    map[string]struct{}
	dispatched map[string]struct{}
	runner     amboy.Runner
	mutex      sync.RWMutex
}

func newRemoteBase() *remoteBase {
	return &remoteBase{
		channel:    make(chan amboy.Job),
		blocked:    make(map[string]struct{}),
		dispatched: make(map[string]struct{}),
	}
}

func (q *remoteBase) ID() string { return q.driver.ID() }

// Put adds a Job to the queue. It is generally an error to add the
// same job to a queue more than once, but this depends on the
// implementation of the underlying driver.
func (q *remoteBase) Put(ctx context.Context, j amboy.Job) error {
	if j.Type().Version < 0 {
		return errors.New("cannot add jobs with versions less than 0")
	}

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job timeinfo")
	}

	return q.driver.Put(ctx, j)
}

// Get retrieves a job from the queue's storage. The second value
// reflects the existence of a job of that name in the queue's
// storage.
func (q *remoteBase) Get(ctx context.Context, name string) (amboy.Job, bool) {
	if q.driver == nil {
		return nil, false
	}

	job, err := q.driver.Get(ctx, name)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"driver": q.driver.ID(),
			"type":   q.driverType,
			"name":   name,
		}))
		return nil, false
	}

	return job, true
}

func (q *remoteBase) jobServer(ctx context.Context) {
	grip.Info("starting queue job server for remote queue")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			job := q.driver.Next(ctx)
			if !q.canDispatch(job) {
				continue
			}

			if !isDispatchable(job.Status()) {
				continue
			}

			// therefore return any pending job or job
			// that has a timed out lock.
			q.channel <- job
		}
	}
}

// Started reports if the queue has begun processing jobs.
func (q *remoteBase) Started() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.started
}

func (q *remoteBase) Save(ctx context.Context, j amboy.Job) error {
	return q.driver.Save(ctx, j)
}

// Complete takes a context and, asynchronously, marks the job
// complete, in the queue.
func (q *remoteBase) Complete(ctx context.Context, j amboy.Job) {
	if ctx.Err() != nil {
		return
	}
	const retryInterval = time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()

	startAt := time.Now()
	id := j.ID()
	count := 0

	var err error
	for {
		count++
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			stat := j.Status()
			stat.Completed = true
			j.SetStatus(stat)

			ti := j.TimeInfo()
			j.UpdateTimeInfo(amboy.JobTimeInfo{
				Start: ti.Start,
				End:   time.Now(),
			})

			err = q.driver.Save(ctx, j)
			if err != nil {
				if time.Since(startAt) > time.Minute+amboy.LockTimeout {
					grip.Error(message.WrapError(err, message.Fields{
						"job_id":      id,
						"job_type":    j.Type().Name,
						"driver_type": q.driverType,
						"retry_count": count,
						"driver_id":   q.driver.ID(),
						"message":     "job took too long to mark complete",
					}))
				} else if count > 10 {
					grip.Error(message.WrapError(err, message.Fields{
						"job_id":      id,
						"driver_type": q.driverType,
						"job_type":    j.Type().Name,
						"driver_id":   q.driver.ID(),
						"retry_count": count,
						"message":     "after 10 retries, aborting marking job complete",
					}))
				} else {
					timer.Reset(retryInterval)
					continue
				}
			}

			j.AddError(err)

			q.mutex.Lock()
			defer q.mutex.Unlock()
			delete(q.blocked, id)
			delete(q.dispatched, id)

			return
		}

	}
}

// Results provides a generator that iterates all completed jobs.
func (q *remoteBase) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)
		for j := range q.driver.Jobs(ctx) {
			if ctx.Err() != nil {
				return
			}
			if j.Status().Completed {
				select {
				case <-ctx.Done():
					return
				case output <- j:
				}
			}
		}
	}()
	return output
}

func (q *remoteBase) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	return q.driver.JobStats(ctx)
}

// Stats returns a amboy. QueueStats object that reflects the progress
// jobs in the queue.
func (q *remoteBase) Stats(ctx context.Context) amboy.QueueStats {
	output := q.driver.Stats(ctx)

	q.mutex.RLock()
	defer q.mutex.RUnlock()
	output.Blocked = len(q.blocked)

	return output
}

// Runner returns (a pointer generally) to the instances' embedded
// amboy.Runner instance. Typically used to call the runner's close
// method.
func (q *remoteBase) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows callers to inject alternate runner implementations
// before starting the queue. After the queue is started it is an
// error to use SetRunner.
func (q *remoteBase) SetRunner(r amboy.Runner) error {
	if q.runner != nil && q.runner.Started() {
		return errors.New("cannot change runners after starting")
	}

	q.runner = r
	return nil
}

// Driver provides access to the embedded driver instance which
// provides access to the Queue's persistence layer. This method is
// not part of the amboy.Queue interface.
func (q *remoteBase) Driver() remoteQueueDriver {
	return q.driver
}

// SetDriver allows callers to inject at runtime alternate driver
// instances. It is an error to change Driver instances after starting
// a queue. This method is not part of the amboy.Queue interface.
func (q *remoteBase) SetDriver(d remoteQueueDriver) error {
	if q.Started() {
		return errors.New("cannot change drivers after starting queue")
	}

	q.driver = d
	q.driverType = fmt.Sprintf("%T", d)
	return nil
}

// Start initiates the job dispatching and prcessing functions of the
// queue. If the queue is started this is a noop, however, if the
// driver or runner are not initialized, this operation returns an
// error. To release the resources created when starting the queue,
// cancel the context used when starting the queue.
func (q *remoteBase) Start(ctx context.Context) error {
	if q.Started() {
		return nil
	}

	if q.driver == nil {
		return errors.New("cannot start queue with an uninitialized driver")
	}

	if q.runner == nil {
		return errors.New("cannot start queue with an uninitialized runner")
	}

	err := q.runner.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting runner in remote queue")
	}

	err = q.driver.Open(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting driver in remote queue")
	}

	go q.jobServer(ctx)
	q.mutex.Lock()
	q.started = true
	q.mutex.Unlock()

	return nil
}

func (q *remoteBase) addBlocked(n string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.blocked[n] = struct{}{}
}

func (q *remoteBase) canDispatch(j amboy.Job) bool {
	if j == nil {
		return false
	}

	id := j.ID()
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, ok := q.dispatched[id]; ok {
		return false
	}

	q.dispatched[id] = struct{}{}
	return true
}

func isDispatchable(stat amboy.JobStatusInfo) bool {
	// don't return completed jobs for any reason
	if stat.Completed {
		return false
	}

	// don't return an inprogress job if the mod
	// time is less than the lock timeout
	if stat.InProgress && time.Since(stat.ModificationTime) < amboy.LockTimeout {
		return false
	}

	return true
}
