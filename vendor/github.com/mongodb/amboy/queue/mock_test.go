package queue

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func init() {
	mockJobCounters = &mockJobRunEnv{}
	registry.AddJobType("mock", func() amboy.Job { return newMockJob() })
	registry.AddJobType("sleep", func() amboy.Job { return newSleepJob() })
	registry.AddJobType("mock-retryable", func() amboy.Job { return makeMockRetryableJob() })
}

// mockRetryableQueue provides a mock implementation of a remoteQueue whose
// runtime behavior is configurable. Methods can be mocked out; otherwise, the
// default behavior is identical to that of its underlying remoteQueue.
type mockRemoteQueue struct {
	remoteQueue

	driver       remoteQueueDriver
	dispatcher   Dispatcher
	retryHandler amboy.RetryHandler

	// Mockable methods
	putJob                    func(ctx context.Context, q remoteQueue, j amboy.Job) error
	getJob                    func(ctx context.Context, q remoteQueue, id string) (amboy.Job, bool)
	getJobAttempt             func(ctx context.Context, q remoteQueue, id string, attempt int) (amboy.Job, error)
	getAllJobAttempts         func(ctx context.Context, q remoteQueue, id string) ([]amboy.Job, error)
	saveJob                   func(ctx context.Context, q remoteQueue, j amboy.Job) error
	completeRetryingAndPutJob func(ctx context.Context, q remoteQueue, toComplete, toPut amboy.Job) error
	nextJob                   func(ctx context.Context, q remoteQueue) amboy.Job
	completeJob               func(ctx context.Context, q remoteQueue, j amboy.Job) error
	completeRetryingJob       func(ctx context.Context, q remoteQueue, j amboy.Job) error
	jobResults                func(ctx context.Context, q remoteQueue) <-chan amboy.Job
	jobInfo                   func(ctx context.Context, q remoteQueue) <-chan amboy.JobInfo
	queueStats                func(ctx context.Context, q remoteQueue) amboy.QueueStats
	queueInfo                 func(q remoteQueue) amboy.QueueInfo
	startQueue                func(ctx context.Context, q remoteQueue) error
	closeQueue                func(ctx context.Context, q remoteQueue)
}

type mockRemoteQueueOptions struct {
	queue            remoteQueue
	driver           remoteQueueDriver
	makeDispatcher   func(q amboy.Queue) Dispatcher
	makeRetryHandler func(q amboy.RetryableQueue) (amboy.RetryHandler, error)
}

func (opts *mockRemoteQueueOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.queue == nil, "cannot initialize mock remote queue without a backing remote queue implementation")
	return catcher.Resolve()
}

func newMockRemoteQueue(opts mockRemoteQueueOptions) (*mockRemoteQueue, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	mq := &mockRemoteQueue{remoteQueue: opts.queue}

	if opts.makeDispatcher != nil {
		mq.dispatcher = opts.makeDispatcher(mq)
	} else {
		mq.dispatcher = newMockDispatcher(mq)
	}

	if opts.driver != nil {
		if err := mq.SetDriver(opts.driver); err != nil {
			return nil, errors.Wrap(err, "configuring queue with driver")
		}
	}

	if opts.queue.RetryHandler() != nil {
		if err := opts.queue.RetryHandler().SetQueue(mq); err != nil {
			return nil, errors.Wrap(err, "attaching mock queue to retry handler")
		}
		mq.retryHandler = opts.queue.RetryHandler()
	}

	return mq, nil
}

func (q *mockRemoteQueue) ID() string { return "mock-remote" }

func (q *mockRemoteQueue) Driver() remoteQueueDriver {
	return q.driver
}

func (q *mockRemoteQueue) SetDriver(d remoteQueueDriver) error {
	if err := q.remoteQueue.SetDriver(d); err != nil {
		return err
	}
	q.driver = d
	q.driver.SetDispatcher(q.dispatcher)
	return nil
}

func (q *mockRemoteQueue) RetryHandler() amboy.RetryHandler {
	return q.retryHandler
}

func (q *mockRemoteQueue) SetRetryHandler(rh amboy.RetryHandler) error {
	q.retryHandler = rh
	return rh.SetQueue(q)
}

func (q *mockRemoteQueue) Put(ctx context.Context, j amboy.Job) error {
	if q.putJob != nil {
		return q.putJob(ctx, q.remoteQueue, j)
	}
	return q.remoteQueue.Put(ctx, j)
}

func (q *mockRemoteQueue) Get(ctx context.Context, id string) (amboy.Job, bool) {
	if q.getJob != nil {
		return q.getJob(ctx, q.remoteQueue, id)
	}
	return q.remoteQueue.Get(ctx, id)
}

func (q *mockRemoteQueue) GetAttempt(ctx context.Context, id string, attempt int) (amboy.Job, error) {
	if q.getJobAttempt != nil {
		return q.getJobAttempt(ctx, q.remoteQueue, id, attempt)
	}
	return q.remoteQueue.GetAttempt(ctx, id, attempt)
}

func (q *mockRemoteQueue) GetAllAttempts(ctx context.Context, id string) ([]amboy.Job, error) {
	if q.getJobAttempt != nil {
		return q.getAllJobAttempts(ctx, q.remoteQueue, id)
	}
	return q.remoteQueue.GetAllAttempts(ctx, id)
}

func (q *mockRemoteQueue) Save(ctx context.Context, j amboy.Job) error {
	if q.saveJob != nil {
		return q.saveJob(ctx, q.remoteQueue, j)
	}
	return q.remoteQueue.Save(ctx, j)
}

func (q *mockRemoteQueue) CompleteRetryingAndPut(ctx context.Context, toComplete, toPut amboy.Job) error {
	if q.completeRetryingAndPutJob != nil {
		return q.completeRetryingAndPutJob(ctx, q.remoteQueue, toComplete, toPut)
	}
	return q.remoteQueue.CompleteRetryingAndPut(ctx, toComplete, toPut)
}

func (q *mockRemoteQueue) Next(ctx context.Context) amboy.Job {
	if q.nextJob != nil {
		return q.nextJob(ctx, q.remoteQueue)
	}
	return q.remoteQueue.Next(ctx)
}

func (q *mockRemoteQueue) Complete(ctx context.Context, j amboy.Job) error {
	if q.completeJob != nil {
		return q.completeJob(ctx, q.remoteQueue, j)
	}
	return q.remoteQueue.Complete(ctx, j)
}

func (q *mockRemoteQueue) CompleteRetrying(ctx context.Context, j amboy.Job) error {
	if q.completeRetryingJob != nil {
		return q.completeRetryingJob(ctx, q.remoteQueue, j)
	}
	return q.remoteQueue.CompleteRetrying(ctx, j)
}

func (q *mockRemoteQueue) Results(ctx context.Context) <-chan amboy.Job {
	if q.jobResults != nil {
		return q.jobResults(ctx, q.remoteQueue)
	}
	return q.remoteQueue.Results(ctx)
}

func (q *mockRemoteQueue) JobInfo(ctx context.Context) <-chan amboy.JobInfo {
	if q.jobInfo != nil {
		return q.jobInfo(ctx, q.remoteQueue)
	}
	return q.remoteQueue.JobInfo(ctx)
}

func (q *mockRemoteQueue) Stats(ctx context.Context) amboy.QueueStats {
	if q.queueStats != nil {
		return q.queueStats(ctx, q.remoteQueue)
	}
	return q.remoteQueue.Stats(ctx)
}

func (q *mockRemoteQueue) Info() amboy.QueueInfo {
	if q.queueInfo != nil {
		return q.queueInfo(q.remoteQueue)
	}
	return q.remoteQueue.Info()
}

func (q *mockRemoteQueue) Start(ctx context.Context) error {
	if q.startQueue != nil {
		return q.startQueue(ctx, q.remoteQueue)
	}
	return q.remoteQueue.Start(ctx)
}

func (q *mockRemoteQueue) Close(ctx context.Context) {
	if q.closeQueue != nil {
		q.closeQueue(ctx, q.remoteQueue)
		return
	}
	q.remoteQueue.Close(ctx)
}

// mockRetryableJob provides a mock implementation of an amboy.Job that is
// retryable by default and whose runtime behavior is configurable.
type mockRetryableJob struct {
	job.Base
	ErrorToAdd          string                 `bson:"error_to_add" json:"error_to_add"`
	RetryableErrorToAdd string                 `bson:"retryable_error_to_add" json:"retryable_error_to_add"`
	NumTimesToRetry     int                    `bson:"num_times_to_retry" json:"num_times_to_retry"`
	UpdatedRetryInfo    *amboy.JobRetryOptions `bson:"updated_retry_info" json:"updated_retry_info"`
	op                  func(*mockRetryableJob)
}

func makeMockRetryableJob() *mockRetryableJob {
	j := &mockRetryableJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "mock-retryable",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func newMockRetryableJob(id string) *mockRetryableJob {
	j := makeMockRetryableJob()
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable: utility.TruePtr(),
	})
	j.SetID(fmt.Sprintf("mock-retryable-%s", id))
	return j
}

func (j *mockRetryableJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.ErrorToAdd != "" {
		j.AddError(errors.New(j.ErrorToAdd))
	}
	if j.RetryableErrorToAdd != "" {
		j.AddRetryableError(errors.New(j.RetryableErrorToAdd))
	}

	if j.UpdatedRetryInfo != nil {
		j.UpdateRetryInfo(*j.UpdatedRetryInfo)
	}

	if j.NumTimesToRetry != 0 && j.RetryInfo().CurrentAttempt < j.NumTimesToRetry {
		j.UpdateRetryInfo(amboy.JobRetryOptions{
			NeedsRetry: utility.TruePtr(),
		})
	}

	if j.op != nil {
		j.op(j)
	}
}

func TestMockRetryableQueue(t *testing.T) {
	assert.Implements(t, (*amboy.RetryableQueue)(nil), &mockRemoteQueue{})
}

var mockJobCounters *mockJobRunEnv

type mockJobRunEnv struct {
	runCount int
	mu       sync.Mutex
}

func (e *mockJobRunEnv) Inc() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runCount++
}

func (e *mockJobRunEnv) Count() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.runCount
}

func (e *mockJobRunEnv) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runCount = 0
}

type mockJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func newMockJob() *mockJob {
	j := &mockJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "mock",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *mockJob) Run(_ context.Context) {
	defer j.MarkComplete()

	mockJobCounters.Inc()
}

type sleepJob struct {
	Sleep time.Duration
	job.Base
}

func newSleepJob() *sleepJob {
	j := &sleepJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "sleep",
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetID(uuid.New().String())
	return j
}

func (j *sleepJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.Sleep == 0 {
		return
	}

	timer := time.NewTimer(j.Sleep)
	defer timer.Stop()

	select {
	case <-timer.C:
		return
	case <-ctx.Done():
		return
	}
}
