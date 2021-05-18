package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// remoteQueue is an interface to an amboy.RetryableQueue that uses a
// remoteQueueDriver to interact with the persistence layer for the queue.
type remoteQueue interface {
	amboy.RetryableQueue
	// SetDriver sets the driver to connect to the persistence layer. The driver
	// must be set before the queue can start. Once the queue has started, the
	// driver cannot be modified.
	SetDriver(remoteQueueDriver) error
	// Driver returns the driver connected to the persistence layer.
	Driver() remoteQueueDriver
}

type remoteBase struct {
	id           string
	opts         remoteOptions
	started      bool
	driver       remoteQueueDriver
	dispatcher   Dispatcher
	driverType   string
	channel      chan amboy.Job
	blocked      map[string]struct{}
	dispatched   map[string]struct{}
	runner       amboy.Runner
	retryHandler amboy.RetryHandler
	cancel       context.CancelFunc
	mutex        sync.RWMutex
}

type remoteOptions struct {
	numWorkers int
	retryable  RetryableQueueOptions
}

func (opts *remoteOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.numWorkers <= 0, "number of workers must be a positive number")
	catcher.Wrap(opts.retryable.Validate(), "invalid retryable queue options")
	return catcher.Resolve()
}

func newRemoteBaseWithOptions(opts remoteOptions) (*remoteBase, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}
	b := &remoteBase{
		id:         uuid.New().String(),
		channel:    make(chan amboy.Job),
		blocked:    make(map[string]struct{}),
		dispatched: make(map[string]struct{}),
		opts:       opts,
	}
	rh, err := NewBasicRetryHandler(b, opts.retryable.RetryHandler)
	if err != nil {
		return nil, errors.Wrap(err, "initializing retry handler")
	}
	b.retryHandler = rh
	return b, nil
}

func (q *remoteBase) ID() string {
	return q.driver.ID()
}

// Put adds a Job to the queue. It is generally an error to add the
// same job to a queue more than once, but this depends on the
// implementation of the underlying driver.
func (q *remoteBase) Put(ctx context.Context, j amboy.Job) error {
	if q.driver == nil {
		return errors.New("driver is not set")
	}
	if err := q.validateAndPreparePut(j); err != nil {
		return err
	}
	return q.driver.Put(ctx, j)
}

func (q *remoteBase) validateAndPreparePut(j amboy.Job) error {
	if j.Type().Version < 0 {
		return errors.New("cannot add jobs with versions less than 0")
	}
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})
	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job time info")
	}
	return nil
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

func (q *remoteBase) GetAttempt(ctx context.Context, id string, attempt int) (amboy.Job, error) {
	if q.driver == nil {
		return nil, errors.New("driver is not set")
	}

	j, err := q.driver.GetAttempt(ctx, id, attempt)
	if err != nil {
		return nil, err
	}

	return j, nil
}

func (q *remoteBase) GetAllAttempts(ctx context.Context, id string) ([]amboy.Job, error) {
	if q.driver == nil {
		return nil, errors.New("driver is not set")
	}

	jobs, err := q.driver.GetAllAttempts(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "")
	}

	return jobs, nil
}

func (q *remoteBase) jobServer(ctx context.Context) {
	grip.Info("starting queue job server for remote queue")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			job := q.driver.Next(ctx)
			if !q.lockDispatch(job) {
				if job != nil {
					q.dispatcher.Release(ctx, job)
					grip.Warning(message.Fields{
						"message":   "releasing a job that's already been dispatched",
						"service":   "amboy.queue.mdb",
						"operation": "post-dispatch lock",
						"job_id":    job.ID(),
						"queue_id":  q.ID(),
					})
				}
				continue
			}

			// Return a successfully dispatched job.
			q.channel <- job
		}
	}
}

func (q *remoteBase) Info() amboy.QueueInfo {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.info()
}

func (q *remoteBase) info() amboy.QueueInfo {
	lockTimeout := amboy.LockTimeout
	if q.driver != nil {
		lockTimeout = q.driver.LockTimeout()
	}
	return amboy.QueueInfo{
		Started:     q.started,
		LockTimeout: lockTimeout,
	}
}

func (q *remoteBase) Save(ctx context.Context, j amboy.Job) error {
	if q.driver == nil {
		return errors.New("driver is not set")
	}
	return q.driver.Save(ctx, j)
}

// Complete marks the job complete in the queue.
func (q *remoteBase) Complete(ctx context.Context, j amboy.Job) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	q.dispatcher.Complete(ctx, j)

	const retryInterval = time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()

	startAt := time.Now()
	id := j.ID()
	count := 0

	catcher := grip.NewBasicCatcher()
	var err error
	for {
		count++
		select {
		case <-ctx.Done():
			catcher.Add(ctx.Err())
			return catcher.Resolve()
		case <-timer.C:
			stat := j.Status()
			stat.Completed = true
			stat.InProgress = false
			j.SetStatus(stat)

			j.UpdateTimeInfo(amboy.JobTimeInfo{
				End: time.Now(),
			})

			err = q.driver.Complete(ctx, j)
			if err != nil {
				if time.Since(startAt) > time.Minute+q.Info().LockTimeout {
					grip.Warning(message.WrapError(err, message.Fields{
						"job_id":      id,
						"job_type":    j.Type().Name,
						"driver_type": q.driverType,
						"retry_count": count,
						"driver_id":   q.driver.ID(),
						"message":     "job took too long to mark complete",
					}))
				} else if count > 10 {
					grip.Warning(message.WrapError(err, message.Fields{
						"job_id":      id,
						"driver_type": q.driverType,
						"job_type":    j.Type().Name,
						"driver_id":   q.driver.ID(),
						"retry_count": count,
						"message":     "after 10 retries, aborting marking job complete",
					}))
				} else if isMongoDupKey(err) || amboy.IsJobNotFoundError(err) {
					grip.Warning(message.WrapError(err, message.Fields{
						"job_id":      id,
						"driver_type": q.driverType,
						"job_type":    j.Type().Name,
						"driver_id":   q.driver.ID(),
						"retry_count": count,
						"message":     "attempting to complete job without lock",
					}))
				} else {
					timer.Reset(retryInterval)
					continue
				}
			}

			catcher.Wrapf(err, "attempt %d", count)
			j.AddError(err)

			q.mutex.Lock()
			defer q.mutex.Unlock()
			delete(q.blocked, id)
			delete(q.dispatched, id)

			if err != nil {
				return catcher.Resolve()
			}

			return nil
		}
	}
}

// CompleteRetryingAndPut marks the job toComplete as finished retrying in the
// queue and adds a new job toPut to the queue. These two operations are atomic.
func (q *remoteBase) CompleteRetryingAndPut(ctx context.Context, toComplete, toPut amboy.Job) error {
	if q.driver == nil {
		return errors.New("driver is not set")
	}

	q.prepareCompleteRetrying(toComplete)
	if err := q.validateAndPreparePut(toPut); err != nil {
		return errors.Wrap(err, "invalid job to put")
	}
	return q.driver.CompleteAndPut(ctx, toComplete, toPut)
}

func (q *remoteBase) prepareCompleteRetrying(j amboy.Job) {
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		NeedsRetry: utility.FalsePtr(),
		End:        utility.ToTimePtr(time.Now()),
	})
}

// CompleteRetrying marks the job as finished retrying in the queue.
func (q *remoteBase) CompleteRetrying(ctx context.Context, j amboy.Job) error {
	if q.driver == nil {
		return errors.New("driver is not set")
	}

	q.prepareCompleteRetrying(j)
	return q.driver.Complete(ctx, j)
}

// Results provides a generator that iterates all completed jobs. Retrying jobs
// are not returned until they finish retrying.
func (q *remoteBase) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)
	go func() {
		defer close(output)
		for j := range q.driver.Jobs(ctx) {
			if ctx.Err() != nil {
				return
			}
			completed := j.Status().Completed && !j.RetryInfo().ShouldRetry()
			if completed {
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

// JobInfo returns a channel that produces information about all jobs in the
// queue. The order in which job information is produced depends on the backing
// storage driver.
func (q *remoteBase) JobInfo(ctx context.Context) <-chan amboy.JobInfo {
	return q.driver.JobInfo(ctx)
}

// Stats returns a amboy.QueueStats object that reflects the progress
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

// RetryHandler provides access to the embedded amboy.RetryHandler for the
// queue.
func (q *remoteBase) RetryHandler() amboy.RetryHandler {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.retryHandler
}

// SetRetryHandler allows callers to inject alternative amboy.RetryHandler
// instances if the queue has not yet started or if the queue is started but
// does not already have a retry handler.
func (q *remoteBase) SetRetryHandler(rh amboy.RetryHandler) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.retryHandler != nil && q.retryHandler.Started() {
		return errors.New("cannot change retry handler after it is already started")
	}
	if err := rh.SetQueue(q); err != nil {
		return err
	}

	q.retryHandler = rh

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
	if q.Info().Started {
		return errors.New("cannot change drivers after starting queue")
	}
	q.driver = d
	q.driver.SetDispatcher(q.dispatcher)
	q.driverType = fmt.Sprintf("%T", d)
	return nil
}

// Start initiates the job dispatching and prcessing functions of the
// queue. If the queue is started this is a noop, however, if the
// driver or runner are not initialized, this operation returns an
// error. To release the resources created when starting the queue,
// cancel the context used when starting the queue.
func (q *remoteBase) Start(ctx context.Context) error {
	if q.Info().Started {
		return nil
	}

	if q.driver == nil {
		return errors.New("cannot start queue with an uninitialized driver")
	}

	if q.runner == nil {
		return errors.New("cannot start queue with an uninitialized runner")
	}

	ctx, q.cancel = context.WithCancel(ctx)

	err := q.runner.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting runner in remote queue")
	}

	err = q.driver.Open(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting driver in remote queue")
	}

	if q.retryHandler != nil {
		if err = q.retryHandler.Start(ctx); err != nil {
			return errors.Wrap(err, "starting retry handler in remote queue")
		}
		go q.monitorStaleRetryingJobs(ctx)
	}

	go q.jobServer(ctx)

	q.started = true

	return nil
}

// Close closes all resources owned by the queue and stops any further
// processing of the queue's work.
func (q *remoteBase) Close(ctx context.Context) {
	if q.cancel != nil {
		q.cancel()
	}
	if q.dispatcher != nil {
		grip.Warning(message.WrapError(q.dispatcher.Close(ctx), message.Fields{
			"message":  "dispatcher closed with errors",
			"service":  "amboy.queue.mdb",
			"queue_id": q.ID(),
		}))
	}
	if q.driver != nil {
		grip.Warning(message.WrapError(q.driver.Close(ctx), message.Fields{
			"message":  "driver closed with errors",
			"service":  "amboy.queue.mdb",
			"queue_id": q.ID(),
		}))
	}
	if r := q.Runner(); r != nil {
		r.Close(ctx)
	}
	if rh := q.RetryHandler(); rh != nil {
		rh.Close(ctx)
	}
}

// Next is a no-op that is included here so that it fulfills the amboy.Queue
// interface.
func (q *remoteBase) Next(context.Context) amboy.Job {
	return nil
}

func (q *remoteBase) addBlocked(n string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.blocked[n] = struct{}{}
}

// lockDispatch attempts to acquire the exclusive lock on a job dispatched by
// this queue. If the job has not yet been dispatched, it marks it as dispatched
// by this queue and returns true. Otherwise, it returns false.
func (q *remoteBase) lockDispatch(j amboy.Job) bool {
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

// defaultStaleRetryingMonitorInterval is the default frequency that an
// amboy.RetryableQueue will check for stale retrying jobs.
const defaultStaleRetryingMonitorInterval = time.Second

func (q *remoteBase) monitorStaleRetryingJobs(ctx context.Context) {
	defer func() {
		if err := recovery.HandlePanicWithError(recover(), nil, "stale retry job monitor"); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "stale retry job monitor failed",
				"service":  "amboy.queue.mdb",
				"queue_id": q.ID(),
			}))
			go q.monitorStaleRetryingJobs(ctx)
		}
	}()

	monitorInterval := defaultStaleRetryingMonitorInterval
	if interval := q.opts.retryable.StaleRetryingMonitorInterval; interval != 0 {
		monitorInterval = interval
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			for j := range q.driver.RetryableJobs(ctx, retryableJobStaleRetrying) {
				grip.Error(message.WrapError(q.retryHandler.Put(ctx, j), message.Fields{
					"message":  "could not enqueue stale retrying job",
					"service":  "amboy.queue.mdb",
					"job_id":   j.ID(),
					"queue_id": q.ID(),
				}))
			}
			timer.Reset(monitorInterval)
		}
	}
}
