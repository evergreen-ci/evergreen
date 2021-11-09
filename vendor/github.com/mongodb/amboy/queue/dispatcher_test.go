package queue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mockDispatcher provides a mock implementation of a Dispatcher whose behavior
// can be more easily configured for testing.
type mockDispatcher struct {
	queue              amboy.Queue
	mu                 sync.Mutex
	dispatched         map[string]dispatcherInfo
	shouldFailDispatch func(amboy.Job) bool
	initialLock        func(amboy.Job) error
	lockPing           func(context.Context, amboy.Job)
	closed             bool
}

func newMockDispatcher(q amboy.Queue) *mockDispatcher {
	return &mockDispatcher{
		queue:      q,
		dispatched: map[string]dispatcherInfo{},
	}
}

func (d *mockDispatcher) Dispatch(ctx context.Context, j amboy.Job) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.shouldFailDispatch != nil && d.shouldFailDispatch(j) {
		return errors.Errorf("fail dispatch for job '%s'", j.ID())
	}

	if _, ok := d.dispatched[j.ID()]; ok {
		return errors.Errorf("cannot dispatch a job more than once")
	}

	ti := amboy.JobTimeInfo{
		Start: time.Now(),
	}
	j.UpdateTimeInfo(ti)

	status := j.Status()
	status.InProgress = true
	j.SetStatus(status)

	if d.initialLock != nil {
		if err := d.initialLock(j); err != nil {
			return errors.Wrap(err, "taking initial lock on job")
		}
	} else {
		if err := j.Lock(d.queue.ID(), d.queue.Info().LockTimeout); err != nil {
			return errors.Wrap(err, "locking job")
		}
		if err := d.queue.Save(ctx, j); err != nil {
			return errors.Wrap(err, "saving job lock")
		}
	}

	pingCtx, pingCancel := context.WithCancel(ctx)
	pingCompleted := make(chan struct{})
	if d.lockPing != nil {
		go func() {
			defer close(pingCompleted)
			defer recovery.LogStackTraceAndContinue("mock background job lock ping", j.ID())
			d.lockPing(pingCtx, j)
		}()
	} else {
		go func() {
			defer close(pingCompleted)
			defer recovery.LogStackTraceAndContinue("mock background job lock ping", j.ID())
			grip.Debug(message.WrapError(pingJobLock(pingCtx, d.queue, j), message.Fields{
				"message":  "could not ping job lock",
				"job_id":   j.ID(),
				"queue_id": d.queue.ID(),
			}))
		}()
	}

	d.dispatched[j.ID()] = dispatcherInfo{
		job:           j,
		pingCtx:       pingCtx,
		pingCancel:    pingCancel,
		pingCompleted: pingCompleted,
	}

	return nil
}

func (d *mockDispatcher) Release(ctx context.Context, j amboy.Job) {
	d.mu.Lock()
	defer d.mu.Unlock()

	grip.Debug(message.WrapError(d.release(ctx, j.ID()), message.Fields{
		"service":  "mock dispatcher",
		"queue_id": d.queue.ID(),
		"job_id":   j.ID(),
	}))
}

func (d *mockDispatcher) release(ctx context.Context, jobID string) error {
	info, ok := d.dispatched[jobID]
	if !ok {
		return errors.New("attempting to release an unowned job")
	}

	delete(d.dispatched, jobID)

	info.pingCancel()

	select {
	case <-ctx.Done():
	case <-info.pingCompleted:
	}

	return nil
}

func (d *mockDispatcher) Complete(ctx context.Context, j amboy.Job) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.release(ctx, j.ID()); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"service":  "mock dispatcher",
			"queue_id": d.queue.ID(),
			"job_id":   j.ID(),
		}))
		return
	}

	ti := j.TimeInfo()
	ti.End = time.Now()
	j.UpdateTimeInfo(ti)
}

func (d *mockDispatcher) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	catcher := grip.NewBasicCatcher()
	for jobID := range d.dispatched {
		catcher.Wrapf(d.release(ctx, jobID), "releasing job '%s'", jobID)
	}

	d.closed = true

	return nil
}

func TestDispatcherImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setLockTimeout := func(lockTimeout time.Duration) func(q remoteQueue) amboy.QueueInfo {
		return func(q remoteQueue) amboy.QueueInfo {
			info := q.Info()
			info.LockTimeout = lockTimeout
			return info
		}
	}

	opts := defaultMongoDBTestOptions()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, client.Disconnect(ctx))
	}()

	driver, err := openNewMongoDriver(ctx, newDriverID(), opts, client)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, driver.Close(ctx))
	}()

	for dispatcherName, makeDispatcher := range map[string]func(q amboy.Queue) Dispatcher{
		"Basic": NewDispatcher,
		"Mock":  func(q amboy.Queue) Dispatcher { return newMockDispatcher(q) },
	} {
		t.Run(dispatcherName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue){
				"DispatchLocksJob": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					require.NoError(t, d.Dispatch(ctx, j))
					defer d.Release(ctx, j)

					status := j.Status()
					assert.NotZero(t, status.ModificationCount)
					assert.NotZero(t, status.ModificationTime)
					assert.True(t, status.InProgress)
				},
				"DispatchPingsLockWithinQueueLockTimeoutInterval": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					require.NoError(t, d.Dispatch(ctx, j))
					defer d.Release(ctx, j)

					oldStatus := j.Status()
					assert.NotZero(t, oldStatus.ModificationCount)
					assert.NotZero(t, oldStatus.ModificationTime)
					assert.True(t, oldStatus.InProgress)

					time.Sleep(2 * lockTimeout)

					newStatus := j.Status()
					assert.True(t, oldStatus.ModificationTime.Before(newStatus.ModificationTime))
					assert.True(t, oldStatus.ModificationCount < newStatus.ModificationCount)
					assert.True(t, newStatus.InProgress)
				},
				"DispatchLockPingerStopsWithContextError": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					cctx, ccancel := context.WithCancel(context.Background())
					require.NoError(t, d.Dispatch(cctx, j))
					defer d.Release(ctx, j)
					ccancel()

					oldStatus := j.Status()
					assert.NotZero(t, oldStatus.ModificationCount)
					assert.NotZero(t, oldStatus.ModificationTime)
					assert.True(t, oldStatus.InProgress)

					time.Sleep(2 * lockTimeout)

					newStatus := j.Status()
					assert.Equal(t, oldStatus.ModificationTime, newStatus.ModificationTime)
					assert.Equal(t, oldStatus.ModificationCount, newStatus.ModificationCount)
					assert.True(t, newStatus.InProgress)
				},
				"DuplicateJobDispatchFails": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					require.NoError(t, d.Dispatch(ctx, j))
					require.Error(t, d.Dispatch(ctx, j))
					defer d.Release(ctx, j)

					oldStatus := j.Status()
					assert.NotZero(t, oldStatus.ModificationCount)
					assert.NotZero(t, oldStatus.ModificationTime)
					assert.True(t, oldStatus.InProgress)

					time.Sleep(2 * lockTimeout)

					newStatus := j.Status()
					assert.True(t, oldStatus.ModificationTime.Before(newStatus.ModificationTime))
					assert.True(t, oldStatus.ModificationCount < newStatus.ModificationCount)
					assert.True(t, newStatus.InProgress)
				},
				"ReleaseStopsLockPinger": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					require.NoError(t, d.Dispatch(ctx, j))
					d.Release(ctx, j)

					oldStatus := j.Status()
					assert.NotZero(t, oldStatus.ModificationCount)
					assert.NotZero(t, oldStatus.ModificationTime)
					assert.True(t, oldStatus.InProgress)

					time.Sleep(2 * lockTimeout)

					newStatus := j.Status()
					assert.Equal(t, oldStatus.ModificationTime, newStatus.ModificationTime)
					assert.Equal(t, oldStatus.ModificationCount, newStatus.ModificationCount)
					assert.True(t, newStatus.InProgress)
				},
				"CompleteStopJobPingAndUpdatesJobInfo": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					require.NoError(t, d.Dispatch(ctx, j))

					oldStatus := j.Status()
					assert.NotZero(t, oldStatus.ModificationCount)
					assert.NotZero(t, oldStatus.ModificationTime)
					assert.True(t, oldStatus.InProgress)

					d.Complete(ctx, j)

					time.Sleep(2 * lockTimeout)

					newStatus := j.Status()
					assert.Equal(t, oldStatus.ModificationTime, newStatus.ModificationTime)
					assert.Equal(t, oldStatus.ModificationCount, newStatus.ModificationCount)
					assert.True(t, newStatus.InProgress)
					assert.NotZero(t, j.TimeInfo().End)

				},
				"CompleteNoopsForUnownedJob": func(ctx context.Context, t *testing.T, d Dispatcher, mq *mockRemoteQueue) {
					lockTimeout := 10 * time.Millisecond
					mq.queueInfo = setLockTimeout(lockTimeout)

					j := newMockJob()
					require.NoError(t, mq.Put(ctx, j))
					d.Complete(ctx, j)

					assert.Zero(t, j.Status().ModificationCount)
					assert.Zero(t, j.Status().ModificationTime)
					assert.False(t, j.Status().InProgress)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
					defer tcancel()

					const size = 10
					q, err := newRemoteUnordered(size)
					require.NoError(t, err)

					require.NoError(t, driver.getCollection().Database().Drop(tctx))
					defer func() {
						assert.NoError(t, driver.getCollection().Database().Drop(tctx))
					}()

					opts := mockRemoteQueueOptions{
						queue:          q,
						driver:         driver,
						makeDispatcher: makeDispatcher,
					}
					mq, err := newMockRemoteQueue(opts)
					require.NoError(t, err)

					testCase(tctx, t, mq.Driver().Dispatcher(), mq)
				})
			}
		})
	}
}
