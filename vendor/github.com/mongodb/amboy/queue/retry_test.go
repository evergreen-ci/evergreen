package queue

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestRetryableQueueOptions(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("SucceedsWithZeroOptions", func(t *testing.T) {
			opts := RetryableQueueOptions{}
			assert.NoError(t, opts.Validate())
		})
		t.Run("FailsWithNegativeStaleRetryingMonitorInterval", func(t *testing.T) {
			opts := RetryableQueueOptions{StaleRetryingMonitorInterval: -time.Second}
			assert.Error(t, opts.Validate())
		})
		t.Run("DefaultsStaleRetryingMonitorInterval", func(t *testing.T) {
			opts := RetryableQueueOptions{StaleRetryingMonitorInterval: 0}
			require.NoError(t, opts.Validate())
			assert.Equal(t, defaultStaleRetryingMonitorInterval, opts.StaleRetryingMonitorInterval)
		})
		t.Run("FailsWithInvalidRetryHandlerOptions", func(t *testing.T) {
			opts := RetryableQueueOptions{
				RetryHandler: amboy.RetryHandlerOptions{
					MaxRetryAttempts: -1,
				},
			}
			assert.Error(t, opts.Validate())
		})
	})
}

func TestNewBasicRetryHandler(t *testing.T) {
	q, err := newRemoteUnordered(1)
	require.NoError(t, err)
	t.Run("SucceedsWithQueue", func(t *testing.T) {
		rh, err := NewBasicRetryHandler(q, amboy.RetryHandlerOptions{})
		assert.NoError(t, err)
		assert.NotZero(t, rh)
	})
	t.Run("FailsWithNilQueue", func(t *testing.T) {
		rh, err := NewBasicRetryHandler(nil, amboy.RetryHandlerOptions{})
		assert.Error(t, err)
		assert.Zero(t, rh)
	})
	t.Run("FailsWithInvalidOptions", func(t *testing.T) {
		rh, err := NewBasicRetryHandler(q, amboy.RetryHandlerOptions{NumWorkers: -1})
		assert.Error(t, err)
		assert.Zero(t, rh)
	})
}

func TestRetryHandlerImplementations(t *testing.T) {
	assert.Implements(t, (*amboy.RetryHandler)(nil), &BasicRetryHandler{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for rhName, makeRetryHandler := range map[string]func(q amboy.RetryableQueue, opts amboy.RetryHandlerOptions) (amboy.RetryHandler, error){
		"Basic": func(q amboy.RetryableQueue, opts amboy.RetryHandlerOptions) (amboy.RetryHandler, error) {
			return NewBasicRetryHandler(q, opts)
		},
	} {
		t.Run(rhName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)){
				"StartFailsWithoutQueue": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.SetQueue(nil))
					assert.Error(t, rh.Start(ctx))
				},
				"IsStartedAfterStartSucceeds": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.Start(ctx))
					assert.True(t, rh.Started())
				},
				"StartIsIdempotent": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.Start(ctx))
					require.True(t, rh.Started())

					require.NoError(t, rh.Start(ctx))
					assert.True(t, rh.Started())
				},
				"CloseSucceedsWithoutFirstStarting": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)
					require.False(t, rh.Started())

					rh.Close(ctx)
					assert.False(t, rh.Started())
				},
				"CloseStopsRetryHandlerAfterStart": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.Start(ctx))
					require.True(t, rh.Started())

					rh.Close(ctx)
					assert.False(t, rh.Started())
				},
				"CloseIsIdempotent": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.Start(ctx))
					require.True(t, rh.Started())

					assert.NotPanics(t, func() {
						rh.Close(ctx)
						assert.False(t, rh.Started())
						rh.Close(ctx)
						assert.False(t, rh.Started())
					})
				},
				"CanRestartAfterClose": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					require.NoError(t, rh.Start(ctx))
					require.True(t, rh.Started())

					rh.Close(ctx)
					require.False(t, rh.Started())

					require.NoError(t, rh.Start(ctx))
					assert.True(t, rh.Started())
				},
				"PutReenqueuesJobWithExpectedState": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					mq, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						NeedsRetry: utility.TruePtr(),
						WaitUntil:  utility.ToTimeDurationPtr(time.Minute),
						DispatchBy: utility.ToTimeDurationPtr(time.Hour),
					})

					var calledGetAttempt, calledSave, calledCompleteRetrying, calledCompleteRetryingAndPut atomic.Value
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						calledGetAttempt.Store(true)
						ji, err := registry.MakeJobInterchange(j, amboy.JSON)
						if err != nil {
							return nil, errors.WithStack(err)
						}
						j, err := ji.Resolve(amboy.JSON)
						if err != nil {
							return nil, errors.WithStack(err)
						}
						return j, nil
					}
					mq.completeRetryingJob = func(context.Context, remoteQueue, amboy.Job) error {
						calledCompleteRetrying.Store(true)
						return nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						calledSave.Store(true)
						return nil
					}
					mq.completeRetryingAndPutJob = func(_ context.Context, _ remoteQueue, toComplete, toPut amboy.Job) error {
						calledCompleteRetryingAndPut.Store(true)

						assert.Zero(t, toComplete.RetryInfo().CurrentAttempt)

						assert.Equal(t, 1, toPut.RetryInfo().CurrentAttempt)
						assert.Zero(t, toPut.TimeInfo().Start)
						assert.Zero(t, toPut.TimeInfo().End)
						assert.WithinDuration(t, time.Now().Add(j.RetryInfo().WaitUntil), toPut.TimeInfo().WaitUntil, time.Second)
						assert.WithinDuration(t, time.Now().Add(j.RetryInfo().DispatchBy), toPut.TimeInfo().DispatchBy, time.Second)
						assert.Zero(t, toPut.Status())

						return nil
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))
					time.Sleep(100 * time.Millisecond)

					called, ok := calledGetAttempt.Load().(bool)
					require.True(t, ok)
					assert.True(t, called)

					called, ok = calledSave.Load().(bool)
					require.True(t, ok)
					assert.True(t, called)

					called, ok = calledCompleteRetryingAndPut.Load().(bool)
					require.True(t, ok)
					assert.True(t, called)

					called, ok = calledCompleteRetrying.Load().(bool)
					require.False(t, ok)
					assert.False(t, called)
				},
				"PutSucceedsButDoesNothingIfUnstarted": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					mq, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)
					var calledMockQueue atomic.Value
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						calledMockQueue.Store(true)
						return nil, errors.New("mock fail")
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						calledMockQueue.Store(true)
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						calledMockQueue.Store(true)
						return nil
					}

					require.False(t, rh.Started())
					require.NoError(t, rh.Put(ctx, newMockRetryableJob("id")))
					time.Sleep(10 * time.Millisecond)

					called, ok := calledMockQueue.Load().(bool)
					require.False(t, ok)
					assert.False(t, called)
				},
				"PutNoopsIfJobDoesNotNeedToRetry": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					mq, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					var getAttemptCalls, saveCalls, completeRetryingCalls, completeRetryingAndPutCalls int64
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						_ = atomic.AddInt64(&getAttemptCalls, 1)
						return j, nil
					}
					mq.completeRetryingJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingCalls, 1)
						return nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&saveCalls, 1)
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingAndPutCalls, 1)
						return errors.New("fail")
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))

					time.Sleep(100 * time.Millisecond)

					assert.NotZero(t, atomic.LoadInt64(&getAttemptCalls))
					assert.Zero(t, atomic.LoadInt64(&saveCalls))
					assert.Zero(t, atomic.LoadInt64(&completeRetryingAndPutCalls))
					assert.NotZero(t, atomic.LoadInt64(&completeRetryingCalls))
				},
				"PutNoopsIfJobUsesAllAttempts": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					mq, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						CurrentAttempt: utility.ToIntPtr(9),
						MaxAttempts:    utility.ToIntPtr(10),
					})
					var getAttemptCalls, saveCalls, completeRetryingCalls, completeRetryingAndPutCalls int64
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						_ = atomic.AddInt64(&getAttemptCalls, 1)
						return j, nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&saveCalls, 1)
						return nil
					}
					mq.completeRetryingJob = func(_ context.Context, _ remoteQueue, j amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingCalls, 1)
						j.UpdateRetryInfo(amboy.JobRetryOptions{
							End: utility.ToTimePtr(time.Now()),
						})
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingAndPutCalls, 1)
						return errors.New("fail")
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))

					jobProcessed := make(chan struct{})
					go func() {
						defer close(jobProcessed)
						for {
							select {
							case <-ctx.Done():
								return
							default:
								if !j.RetryInfo().End.IsZero() {
									return
								}
							}
						}
					}()

					select {
					case <-ctx.Done():
						require.FailNow(t, "context is done before job could be processed")
					case <-jobProcessed:
						assert.NotZero(t, j.RetryInfo().End)
						assert.NotZero(t, atomic.LoadInt64(&getAttemptCalls))
						assert.Zero(t, atomic.LoadInt64(&completeRetryingAndPutCalls))
						assert.NotZero(t, atomic.LoadInt64(&completeRetryingCalls))
					}
				},
				"PutFailsWithNilJob": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					_, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{})
					require.NoError(t, err)

					assert.Error(t, rh.Put(ctx, nil))
				},
				"MaxRetryAttemptsLimitsEnqueueAttempts": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					opts := amboy.RetryHandlerOptions{
						RetryBackoff:     10 * time.Millisecond,
						MaxRetryAttempts: 3,
					}
					mq, rh, err := makeQueueAndRetryHandler(opts)
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						NeedsRetry: utility.TruePtr(),
					})

					var getAttemptCalls, saveCalls, completeRetryingCalls, completeRetryingAndPutCalls int64
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						_ = atomic.AddInt64(&getAttemptCalls, 1)
						return j, nil
					}
					mq.completeRetryingJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingCalls, 1)
						return nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&saveCalls, 1)
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingAndPutCalls, 1)
						return errors.New("fail")
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))

					time.Sleep(3 * opts.RetryBackoff * time.Duration(opts.MaxRetryAttempts))

					assert.Equal(t, opts.MaxRetryAttempts, int(atomic.LoadInt64(&getAttemptCalls)))
					assert.Equal(t, opts.MaxRetryAttempts, int(atomic.LoadInt64(&saveCalls)))
					assert.Equal(t, opts.MaxRetryAttempts, int(atomic.LoadInt64(&completeRetryingAndPutCalls)))
					assert.NotZero(t, atomic.LoadInt64(&completeRetryingCalls))
				},
				"RetryBackoffWaitsBeforeAttemptingReenqueue": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					opts := amboy.RetryHandlerOptions{
						RetryBackoff:     10 * time.Millisecond,
						MaxRetryAttempts: 20,
					}
					mq, rh, err := makeQueueAndRetryHandler(opts)
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						NeedsRetry: utility.TruePtr(),
					})

					var getAttemptCalls, saveCalls, completeRetryingCalls, completeRetryingAndPutCalls int64
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						_ = atomic.AddInt64(&getAttemptCalls, 1)
						return j, nil
					}
					mq.completeRetryingJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingCalls, 1)
						return nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&saveCalls, 1)
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingAndPutCalls, 1)
						return errors.New("fail")
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))

					time.Sleep(time.Duration(opts.MaxRetryAttempts) * opts.RetryBackoff / 2)

					assert.True(t, atomic.LoadInt64(&getAttemptCalls) > 1, "worker should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&getAttemptCalls)) < opts.MaxRetryAttempts, "worker should not have used up all attempts")
					assert.True(t, atomic.LoadInt64(&saveCalls) > 1, "worker should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&saveCalls)) < opts.MaxRetryAttempts, "worker should not have used up all attempts")
					assert.True(t, atomic.LoadInt64(&completeRetryingAndPutCalls) > 1, "workers should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&completeRetryingAndPutCalls)) < opts.MaxRetryAttempts, "worker should not have used up all attempts")
					assert.Zero(t, atomic.LoadInt64(&completeRetryingCalls), "workers should not have used up all attempts")
				},
				"MaxRetryTimeStopsEnqueueAttemptsEarly": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error)) {
					opts := amboy.RetryHandlerOptions{
						RetryBackoff:     10 * time.Millisecond,
						MaxRetryTime:     20 * time.Millisecond,
						MaxRetryAttempts: 5,
					}
					mq, rh, err := makeQueueAndRetryHandler(opts)
					require.NoError(t, err)

					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						NeedsRetry: utility.TruePtr(),
					})

					var getAttemptCalls, saveCalls, completeRetryingCalls, completeRetryingAndPutCalls int64
					mq.getJobAttempt = func(context.Context, remoteQueue, string, int) (amboy.Job, error) {
						_ = atomic.AddInt64(&getAttemptCalls, 1)
						return j, nil
					}
					mq.completeRetryingJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingCalls, 1)
						return nil
					}
					mq.saveJob = func(context.Context, remoteQueue, amboy.Job) error {
						_ = atomic.AddInt64(&saveCalls, 1)
						return nil
					}
					mq.completeRetryingAndPutJob = func(context.Context, remoteQueue, amboy.Job, amboy.Job) error {
						_ = atomic.AddInt64(&completeRetryingAndPutCalls, 1)
						return errors.New("fail")
					}

					require.NoError(t, rh.Start(ctx))
					require.NoError(t, rh.Put(ctx, j))

					time.Sleep(time.Duration(opts.MaxRetryAttempts) * opts.RetryBackoff)

					assert.True(t, atomic.LoadInt64(&getAttemptCalls) > 1, "worker should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&getAttemptCalls)) < opts.MaxRetryAttempts, "worker should have aborted early before using up all attempts")
					assert.True(t, atomic.LoadInt64(&saveCalls) > 1, "worker should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&saveCalls)) < (opts.MaxRetryAttempts), "worker should not have used up all attempts")
					assert.True(t, atomic.LoadInt64(&completeRetryingAndPutCalls) > 1, "workers should have had time to attempt more than once")
					assert.True(t, int(atomic.LoadInt64(&completeRetryingAndPutCalls)) < opts.MaxRetryAttempts, "worker should have aborted early before using up all attempts")
					assert.NotZero(t, atomic.LoadInt64(&completeRetryingCalls), "worker should have aborted early")
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
					defer tcancel()

					makeQueueAndRetryHandler := func(opts amboy.RetryHandlerOptions) (*mockRemoteQueue, amboy.RetryHandler, error) {
						q, err := newRemoteUnorderedWithOptions(remoteOptions{
							numWorkers: 10,
							retryable:  RetryableQueueOptions{RetryHandler: opts},
						})
						require.NoError(t, err)

						mqOpts := mockRemoteQueueOptions{
							queue:          q,
							makeDispatcher: NewDispatcher,
						}
						mq, err := newMockRemoteQueue(mqOpts)
						if err != nil {
							return nil, nil, errors.WithStack(err)
						}
						rh, err := makeRetryHandler(mq, opts)
						if err != nil {
							return nil, nil, errors.WithStack(err)
						}
						if err := mq.SetRetryHandler(rh); err != nil {
							return nil, nil, errors.WithStack(err)
						}

						return mq, mq.RetryHandler(), nil
					}

					testCase(tctx, t, makeQueueAndRetryHandler)
				})
			}
		})
	}
}

func defaultRetryableQueueTestCases(d remoteQueueDriver) map[string]func(size int) (amboy.RetryableQueue, error) {
	return map[string]func(size int) (amboy.RetryableQueue, error){
		"RemoteUnordered": func(size int) (amboy.RetryableQueue, error) {
			q, err := newRemoteUnordered(size)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if err := q.SetDriver(d); err != nil {
				return nil, errors.WithStack(err)
			}
			return q, nil
		},
		"RemoteOrdered": func(size int) (amboy.RetryableQueue, error) {
			q, err := newRemoteSimpleOrdered(size)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			if err := q.SetDriver(d); err != nil {
				return nil, errors.WithStack(err)
			}
			return q, nil
		},
		"LocalLimitedSizeSerializable": func(size int) (amboy.RetryableQueue, error) {
			return NewLocalLimitedSizeSerializable(size, size)
		},
	}
}

// TestRetryableQueueImplementations tests functionality specific to the
// amboy.RetryableQueue interface.
func TestRetryableQueueImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	const queueSize = 16

	for queueName, makeQueue := range defaultRetryableQueueTestCases(driver) {
		t.Run(queueName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, q amboy.RetryableQueue){
				"GetAttemptReturnsMatchingJob": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					const (
						numJobs     = 3
						numAttempts = 5
					)
					jobs := map[string]amboy.Job{}
					for jobNum := 0; jobNum < numJobs; jobNum++ {
						for attempt := 0; attempt < numAttempts; attempt++ {
							j := newMockRetryableJob(strconv.Itoa(jobNum))
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								CurrentAttempt: utility.ToIntPtr(attempt),
							})
							jobs[j.ID()+strconv.Itoa(attempt)] = j
							require.NoError(t, q.Put(ctx, j))
						}
					}

					for _, j := range jobs {
						found, err := q.GetAttempt(ctx, j.ID(), j.RetryInfo().CurrentAttempt)
						require.NoError(t, err)
						assert.Equal(t, j.RetryInfo(), found.RetryInfo())
					}
				},
				"GetAttemptFailsWithNonexistentJobID": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					found, err := q.GetAttempt(ctx, "nonexistent", 0)
					assert.Error(t, err)
					assert.Zero(t, found)
				},
				"GetAttemptFailsWithNonexistentJobAttempt": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j := newMockRetryableJob("id")
					require.NoError(t, q.Put(ctx, j))
					found, err := q.GetAttempt(ctx, j.ID(), 1)
					assert.Error(t, err)
					assert.Zero(t, found)
				},
				"GetAttemptFailsWithoutRetryableJob": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j := newMockJob()
					require.NoError(t, q.Put(ctx, j))
					found, err := q.GetAttempt(ctx, j.ID(), 0)
					assert.Error(t, err)
					assert.Zero(t, found)
				},
				"GetAllAttemptReturnsMatchingJobAttemptSequence": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					const (
						numJobs     = 3
						numAttempts = 5
					)
					jobs := map[string][]amboy.Job{}
					for jobNum := 0; jobNum < numJobs; jobNum++ {
						for attempt := 0; attempt < numAttempts; attempt++ {
							j := newMockRetryableJob(strconv.Itoa(jobNum))
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								CurrentAttempt: utility.ToIntPtr(attempt),
								MaxAttempts:    utility.ToIntPtr(numAttempts),
							})
							jobs[j.ID()] = append(jobs[j.ID()], j)
							require.NoError(t, q.Put(ctx, j))
						}
					}

					for id := range jobs {
						found, err := q.GetAllAttempts(ctx, id)
						require.NoError(t, err)
						require.Len(t, found, len(jobs[id]))
						for i := range jobs[id] {
							assert.Equal(t, found[i].RetryInfo(), jobs[id][i].RetryInfo())
							assert.Equal(t, found[i].ID(), jobs[id][i].ID())
						}
					}
				},
				"GetAllAttemptsFailsWithNonexistentJobID": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					found, err := q.GetAttempt(ctx, "nonexistent", 0)
					assert.Error(t, err)
					assert.Zero(t, found)
				},
				"GetAllAttemptsFailsWithoutRetryableJob": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j := newMockJob()
					require.NoError(t, q.Put(ctx, j))
					found, err := q.GetAllAttempts(ctx, j.ID())
					assert.Error(t, err)
					assert.Zero(t, found)
				},
				"CompleteRetryingSucceeds": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j := newMockRetryableJob("id")
					j.UpdateRetryInfo(amboy.JobRetryOptions{
						Start:      utility.ToTimePtr(time.Now()),
						NeedsRetry: utility.TruePtr(),
					})
					require.NoError(t, q.Put(ctx, j))

					require.NoError(t, q.CompleteRetrying(ctx, j))

					assert.False(t, j.RetryInfo().NeedsRetry)
					assert.NotZero(t, j.RetryInfo().End)
					found, err := q.GetAttempt(ctx, j.ID(), 0)
					require.NoError(t, err)
					assert.Equal(t, bsonJobRetryInfo(j.RetryInfo()), bsonJobRetryInfo(found.RetryInfo()))
				},
				"CompleteRetryingAndPutSucceeds": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j0 := newMockRetryableJob("id0")
					j0.UpdateRetryInfo(amboy.JobRetryOptions{
						MaxAttempts: utility.ToIntPtr(10),
					})
					j1 := newMockRetryableJob("id1")
					j1.UpdateRetryInfo(amboy.JobRetryOptions{
						MaxAttempts: utility.ToIntPtr(20),
					})

					require.NoError(t, q.Put(ctx, j0))
					require.NoError(t, q.CompleteRetryingAndPut(ctx, j0, j1))

					found0, err := q.GetAttempt(ctx, j0.ID(), 0)
					require.NoError(t, err)
					assert.Equal(t, bsonJobRetryInfo(j0.RetryInfo()), bsonJobRetryInfo(found0.RetryInfo()))

					found1, err := q.GetAttempt(ctx, j1.ID(), 0)
					require.NoError(t, err)
					assert.Equal(t, bsonJobRetryInfo(j1.RetryInfo()), bsonJobRetryInfo(found1.RetryInfo()))
				},
				"CompleteRetryingFailsWithoutJobInQueue": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j := newMockRetryableJob("id")
					require.Error(t, q.CompleteRetrying(ctx, j))
					found, err := q.GetAttempt(ctx, j.ID(), 0)
					assert.True(t, amboy.IsJobNotFoundError(err))
					assert.Zero(t, found)
				},
				"CompleteRetryingAndPutFailsWithoutJobToCompleteInQueue": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j0 := newMockRetryableJob("id0")
					j1 := newMockRetryableJob("id1")

					require.Error(t, q.CompleteRetryingAndPut(ctx, j0, j1))

					found0, err := q.GetAttempt(ctx, j0.ID(), 0)
					assert.True(t, amboy.IsJobNotFoundError(err))
					assert.Zero(t, found0)

					found1, err := q.GetAttempt(ctx, j1.ID(), 0)
					assert.True(t, amboy.IsJobNotFoundError(err))
					assert.Zero(t, found1)
				},
				"CompleteRetryingAndPutFailsWithJobToPutAlreadyInQueue": func(ctx context.Context, t *testing.T, q amboy.RetryableQueue) {
					j0 := newMockRetryableJob("id0")
					j1 := newMockRetryableJob("id1")

					require.NoError(t, q.Put(ctx, j0))
					require.NoError(t, q.Put(ctx, j1))
					now := time.Now()
					ti := amboy.JobTimeInfo{
						Start: now,
						End:   now,
					}
					j0.SetTimeInfo(ti)
					j1.SetTimeInfo(ti)

					require.Error(t, q.CompleteRetryingAndPut(ctx, j0, j1))

					found0, err := q.GetAttempt(ctx, j0.ID(), 0)
					require.NoError(t, err)
					assert.Zero(t, found0.TimeInfo().Start)
					assert.Zero(t, found0.TimeInfo().End)

					found1, err := q.GetAttempt(ctx, j1.ID(), 0)
					require.NoError(t, err)
					assert.Zero(t, found1.TimeInfo().Start)
					assert.Zero(t, found1.TimeInfo().End)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
					defer tcancel()

					require.NoError(t, driver.getCollection().Database().Drop(tctx))
					defer func() {
						require.NoError(t, driver.getCollection().Database().Drop(tctx))
					}()

					const size = 128
					q, err := makeQueue(size)
					require.NoError(t, err)

					testCase(tctx, t, q)
				})
			}
		})
	}
}

// TestRetryHandlerQueueIntegration tests the functionality provided by an
// amboy.RetryableQueue and an amboy.RetryHandler for retryable jobs.
func TestRetryHandlerQueueIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := defaultMongoDBTestOptions()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI))
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, client.Disconnect(ctx))
	}()

	driver, err := openNewMongoDriver(ctx, newDriverID(), opts, client)
	require.NoError(t, err)

	require.NoError(t, driver.Open(ctx))
	defer func() {
		assert.NoError(t, driver.Close(ctx))
	}()

	for rhName, makeRetryHandler := range map[string]func(q amboy.RetryableQueue, opts amboy.RetryHandlerOptions) (amboy.RetryHandler, error){
		"Basic": func(q amboy.RetryableQueue, opts amboy.RetryHandlerOptions) (amboy.RetryHandler, error) {
			return NewBasicRetryHandler(q, opts)
		},
	} {
		t.Run(rhName, func(t *testing.T) {
			for queueName, makeQueue := range defaultRetryableQueueTestCases(driver) {
				t.Run(queueName, func(t *testing.T) {
					for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)){
						"RetryingSucceedsAndQueueHasExpectedState": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry: utility.TruePtr(),
							})
							now := utility.BSONTime(time.Now())
							j.UpdateTimeInfo(amboy.JobTimeInfo{
								Start:      now,
								End:        now,
								WaitUntil:  now.Add(time.Hour),
								DispatchBy: now.Add(2 * time.Hour),
								MaxTime:    time.Minute,
							})
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, q.Put(ctx, j))

							require.NoError(t, rh.Put(ctx, j))

							jobRetried := make(chan struct{})
							go func() {
								defer close(jobRetried)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if _, err := q.GetAttempt(ctx, j.ID(), 1); err == nil {
											return
										}
									}
								}
							}()

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-jobRetried:
								oldJob, err := q.GetAttempt(ctx, j.ID(), 0)
								require.NoError(t, err, "old job should still exist in the queue")

								oldRetryInfo := oldJob.RetryInfo()
								assert.True(t, oldRetryInfo.Retryable)
								assert.False(t, oldRetryInfo.NeedsRetry)
								assert.Zero(t, oldRetryInfo.CurrentAttempt)
								assert.NotZero(t, oldRetryInfo.Start)
								assert.NotZero(t, oldRetryInfo.End)
								assert.Equal(t, bsonJobStatusInfo(j.Status()), bsonJobStatusInfo(oldJob.Status()))
								assert.Equal(t, bsonJobTimeInfo(j.TimeInfo()), bsonJobTimeInfo(oldJob.TimeInfo()))

								newJob, err := q.GetAttempt(ctx, j.ID(), 1)
								require.NoError(t, err, "new job should have been enqueued")

								newRetryInfo := newJob.RetryInfo()
								assert.True(t, newRetryInfo.Retryable)
								assert.False(t, newRetryInfo.NeedsRetry)
								assert.Equal(t, 1, newRetryInfo.CurrentAttempt)
								assert.Zero(t, newRetryInfo.Start)
								assert.Zero(t, newRetryInfo.End)

								newStatus := newJob.Status()
								assert.False(t, newStatus.InProgress)
								assert.False(t, newStatus.Completed)
								assert.Zero(t, newStatus.Owner)
								assert.Zero(t, newStatus.ModificationCount)
								assert.Zero(t, newStatus.ModificationTime)
								assert.Zero(t, newStatus.Errors)
								assert.Zero(t, newStatus.ErrorCount)

								newTimeInfo := newJob.TimeInfo()
								assert.NotZero(t, newTimeInfo.Created)
								assert.Zero(t, newTimeInfo.Start)
								assert.Zero(t, newTimeInfo.End)
								assert.Equal(t, j.TimeInfo().DispatchBy, utility.BSONTime(newTimeInfo.DispatchBy))
								assert.Equal(t, j.TimeInfo().WaitUntil, utility.BSONTime(newTimeInfo.WaitUntil))
								assert.Equal(t, j.TimeInfo().MaxTime, newTimeInfo.MaxTime)
							}
						},
						"RetryingSucceedsWithScopedJob": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							scopes := []string{"scope"}
							j.SetScopes(scopes)
							j.SetShouldApplyScopesOnEnqueue(true)
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry: utility.TruePtr(),
							})
							now := utility.BSONTime(time.Now())
							j.UpdateTimeInfo(amboy.JobTimeInfo{
								Start:      now,
								End:        now,
								WaitUntil:  now.Add(time.Hour),
								DispatchBy: now.Add(2 * time.Hour),
								MaxTime:    time.Minute,
							})
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, q.Put(ctx, j))
							require.Equal(t, 1, q.Stats(ctx).Total)

							require.NoError(t, rh.Put(ctx, j))

							jobRetried := make(chan struct{})
							go func() {
								defer close(jobRetried)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if _, err := q.GetAttempt(ctx, j.ID(), 1); err == nil {
											return
										}
									}
								}
							}()

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-jobRetried:
								oldJob, err := q.GetAttempt(ctx, j.ID(), 0)
								require.NoError(t, err, "old job should still exist in the queue")

								oldRetryInfo := oldJob.RetryInfo()
								assert.True(t, oldRetryInfo.Retryable)
								assert.False(t, oldRetryInfo.NeedsRetry)
								assert.Zero(t, oldRetryInfo.CurrentAttempt)
								assert.NotZero(t, oldRetryInfo.Start)
								assert.NotZero(t, oldRetryInfo.End)
								assert.Equal(t, scopes, oldJob.Scopes())
								assert.Equal(t, bsonJobStatusInfo(j.Status()), bsonJobStatusInfo(oldJob.Status()))
								assert.Equal(t, bsonJobTimeInfo(j.TimeInfo()), bsonJobTimeInfo(oldJob.TimeInfo()))

								newJob, err := q.GetAttempt(ctx, j.ID(), 1)
								require.NoError(t, err, "new job should have been enqueued")

								newRetryInfo := newJob.RetryInfo()
								assert.True(t, newRetryInfo.Retryable)
								assert.False(t, newRetryInfo.NeedsRetry)
								assert.Equal(t, 1, newRetryInfo.CurrentAttempt)
								assert.Zero(t, newRetryInfo.Start)
								assert.Zero(t, newRetryInfo.End)

								newStatus := newJob.Status()
								assert.False(t, newStatus.InProgress)
								assert.False(t, newStatus.Completed)
								assert.Zero(t, newStatus.Owner)
								assert.Zero(t, newStatus.ModificationCount)
								assert.Zero(t, newStatus.ModificationTime)
								assert.Zero(t, newStatus.Errors)
								assert.Zero(t, newStatus.ErrorCount)

								newTimeInfo := newJob.TimeInfo()
								assert.NotZero(t, newTimeInfo.Created)
								assert.Zero(t, newTimeInfo.Start)
								assert.Zero(t, newTimeInfo.End)
								assert.Equal(t, scopes, newJob.Scopes())
								assert.Equal(t, j.TimeInfo().DispatchBy, utility.BSONTime(newTimeInfo.DispatchBy))
								assert.Equal(t, j.TimeInfo().WaitUntil, utility.BSONTime(newTimeInfo.WaitUntil))
								assert.Equal(t, j.TimeInfo().MaxTime, newTimeInfo.MaxTime)
							}
						},
						"RetryingPopulatesOptionsInNewJob": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							now := utility.BSONTime(time.Now())
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry:  utility.TruePtr(),
								MaxAttempts: utility.ToIntPtr(5),
								WaitUntil:   utility.ToTimeDurationPtr(time.Hour),
								DispatchBy:  utility.ToTimeDurationPtr(2 * time.Hour),
							})
							j.UpdateTimeInfo(amboy.JobTimeInfo{
								Start:      now,
								End:        now,
								WaitUntil:  now.Add(-time.Minute),
								DispatchBy: now.Add(2 * time.Minute),
								MaxTime:    time.Minute,
							})
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, q.Put(ctx, j))

							require.NoError(t, rh.Put(ctx, j))

							jobRetried := make(chan struct{})
							go func() {
								defer close(jobRetried)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if _, err := q.GetAttempt(ctx, j.ID(), 1); err == nil {
											return
										}
									}
								}
							}()

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-jobRetried:
								oldJob, err := q.GetAttempt(ctx, j.ID(), 0)
								require.NoError(t, err, "old job should still exist in the queue")
								assert.False(t, oldJob.RetryInfo().NeedsRetry)
								assert.Zero(t, oldJob.RetryInfo().CurrentAttempt)
								assert.NotZero(t, oldJob.RetryInfo().Start)
								assert.NotZero(t, oldJob.RetryInfo().End)
								assert.Equal(t, bsonJobStatusInfo(j.Status()), bsonJobStatusInfo(oldJob.Status()))
								assert.Equal(t, bsonJobTimeInfo(j.TimeInfo()), bsonJobTimeInfo(oldJob.TimeInfo()))

								newJob, err := q.GetAttempt(ctx, j.ID(), 1)
								require.NoError(t, err, "new job should have been enqueued")

								assert.True(t, newJob.RetryInfo().Retryable)
								assert.False(t, newJob.RetryInfo().NeedsRetry)
								assert.Equal(t, 1, newJob.RetryInfo().CurrentAttempt)
								assert.Zero(t, newJob.RetryInfo().Start)
								assert.Zero(t, newJob.RetryInfo().End)
								assert.Equal(t, j.RetryInfo().MaxAttempts, newJob.RetryInfo().MaxAttempts)
								assert.Equal(t, j.RetryInfo().DispatchBy, newJob.RetryInfo().DispatchBy)
								assert.Equal(t, j.RetryInfo().WaitUntil, newJob.RetryInfo().WaitUntil)

								assert.NotZero(t, newJob.TimeInfo().Created)
								assert.Zero(t, newJob.TimeInfo().Start)
								assert.Zero(t, newJob.TimeInfo().End)
								assert.NotEqual(t, j.TimeInfo().DispatchBy, utility.BSONTime(newJob.TimeInfo().DispatchBy))
								assert.WithinDuration(t, now.Add(j.RetryInfo().DispatchBy), newJob.TimeInfo().DispatchBy, time.Minute)
								assert.NotEqual(t, j.TimeInfo().WaitUntil, utility.BSONTime(newJob.TimeInfo().WaitUntil))
								assert.WithinDuration(t, now.Add(j.RetryInfo().WaitUntil), newJob.TimeInfo().WaitUntil, time.Minute)
								assert.Equal(t, j.TimeInfo().MaxTime, newJob.TimeInfo().MaxTime)
							}
						},
						"RetryingFailsIfJobIsNotInQueue": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							now := utility.BSONTime(time.Now())
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry: utility.TruePtr(),
							})
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, rh.Put(ctx, j))

							jobProcessed := make(chan struct{})
							go func() {
								defer close(jobProcessed)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if !j.RetryInfo().End.IsZero() {
											return
										}
									}
								}
							}()

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-jobProcessed:
								assert.Zero(t, q.Stats(ctx).Total, "queue state should not be modified when retrying job is missing from queue")
							}
						},
						"RetryNoopsIfJobRetryIsAlreadyInQueue": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							now := utility.BSONTime(time.Now())
							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry:     utility.TruePtr(),
								CurrentAttempt: utility.ToIntPtr(0),
							})
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, q.Put(ctx, j))

							ji, err := registry.MakeJobInterchange(j, amboy.JSON)
							require.NoError(t, err)
							retryJob, err := ji.Resolve(amboy.JSON)
							require.NoError(t, err)
							retryJob.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry:     utility.FalsePtr(),
								CurrentAttempt: utility.ToIntPtr(1),
							})

							require.NoError(t, q.Put(ctx, retryJob))

							retryProcessed := make(chan struct{})
							go func() {
								defer close(retryProcessed)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if j, err := q.GetAttempt(ctx, j.ID(), 0); err == nil && !j.RetryInfo().End.IsZero() {
											return
										}
									}
								}
							}()

							require.NoError(t, rh.Put(ctx, j))

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-retryProcessed:
								assert.Equal(t, 2, q.Stats(ctx).Total)
								storedJob, err := q.GetAttempt(ctx, j.ID(), 1)
								require.NoError(t, err, "new job should be enqueued")
								assert.Equal(t, bsonJobTimeInfo(retryJob.TimeInfo()), bsonJobTimeInfo(storedJob.TimeInfo()))
								assert.Equal(t, bsonJobStatusInfo(retryJob.Status()), bsonJobStatusInfo(storedJob.Status()))
								assert.Equal(t, retryJob.RetryInfo(), retryJob.RetryInfo())
							}
						},
						"RetryNoopsIfJobInQueueDoesNotNeedToRetry": func(ctx context.Context, t *testing.T, makeQueueAndRetryHandler func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error)) {
							q, rh, err := makeQueueAndRetryHandler(amboy.RetryHandlerOptions{
								MaxRetryAttempts: 1,
							})
							require.NoError(t, err)

							require.NoError(t, rh.Start(ctx))

							j := newMockRetryableJob("id")
							now := utility.BSONTime(time.Now())
							j.SetStatus(amboy.JobStatusInfo{
								Completed:         true,
								ModificationCount: 10,
								ModificationTime:  now,
								Owner:             q.ID(),
							})

							require.NoError(t, q.Put(ctx, j))

							j.UpdateRetryInfo(amboy.JobRetryOptions{
								NeedsRetry: utility.TruePtr(),
							})

							retryProcessed := make(chan struct{})
							go func() {
								defer close(retryProcessed)
								for {
									select {
									case <-ctx.Done():
										return
									default:
										if j, err := q.GetAttempt(ctx, j.ID(), 0); err == nil && !j.RetryInfo().End.IsZero() {
											return
										}
									}
								}
							}()

							require.NoError(t, rh.Put(ctx, j))

							select {
							case <-ctx.Done():
								require.FailNow(t, "context is done before job could retry")
							case <-retryProcessed:
								assert.Equal(t, 1, q.Stats(ctx).Total)
								storedJob, err := q.GetAttempt(ctx, j.ID(), 0)
								require.NoError(t, err, "job should still exist in the queue")
								assert.False(t, storedJob.RetryInfo().NeedsRetry)

								_, err = q.GetAttempt(ctx, j.ID(), 1)
								assert.True(t, amboy.IsJobNotFoundError(err), "job should not have retried")
							}
						},
					} {
						t.Run(testName, func(t *testing.T) {
							tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
							defer tcancel()

							require.NoError(t, driver.getCollection().Database().Drop(tctx))
							defer func() {
								assert.NoError(t, driver.getCollection().Database().Drop(tctx))
							}()

							makeQueueAndRetryHandler := func(opts amboy.RetryHandlerOptions) (amboy.RetryableQueue, amboy.RetryHandler, error) {
								q, err := makeQueue(10)
								if err != nil {
									return nil, nil, errors.WithStack(err)
								}
								if rq, ok := q.(remoteQueue); ok {
									if err := rq.SetDriver(driver); err != nil {
										return nil, nil, errors.WithStack(err)
									}
								}
								rh, err := makeRetryHandler(q, opts)
								if err != nil {
									return nil, nil, errors.WithStack(err)
								}
								return q, rh, nil
							}

							testCase(tctx, t, makeQueueAndRetryHandler)
						})
					}
				})
			}
		})
	}
}
