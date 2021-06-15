package queue

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultLocalQueueCapcity = 10000

func init() {
	grip.SetName("amboy.queue.tests")
	grip.Error(grip.SetSender(send.MakeNative()))

	lvl := grip.GetSender().Level()
	lvl.Threshold = level.Error
	_ = grip.GetSender().SetLevel(lvl)

	job.RegisterDefaultJobs()
}

func newDriverID() string { return strings.Replace(uuid.New().String(), "-", ".", -1) }

type TestCloser func(context.Context) error

type QueueTestCase struct {
	Name                    string
	Constructor             func(context.Context, string, int) (amboy.Queue, TestCloser, error)
	MinSize                 int
	MaxSize                 int
	SingleWorker            bool
	MultiSupported          bool
	OrderedSupported        bool
	OrderedStartsBefore     bool
	WaitUntilSupported      bool
	DispatchBeforeSupported bool
	MaxTimeSupported        bool
	ScopesSupported         bool
	RetrySupported          bool
	SkipUnordered           bool
	IsRemote                bool
	Skip                    bool
}

type PoolTestCase struct {
	Name         string
	SetPool      func(amboy.Queue, int) error
	SkipRemote   bool
	SkipMulti    bool
	RateLimiting bool
	MinSize      int
	MaxSize      int
}

type SizeTestCase struct {
	Name string
	Size int
}

func DefaultQueueTestCases() []QueueTestCase {
	return []QueueTestCase{
		{
			Name:                    "AdaptiveOrdering",
			OrderedSupported:        true,
			OrderedStartsBefore:     true,
			WaitUntilSupported:      true,
			SingleWorker:            true,
			DispatchBeforeSupported: true,
			MaxTimeSupported:        true,
			MinSize:                 2,
			MaxSize:                 16,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewAdaptiveOrderedLocalQueue(size, defaultLocalQueueCapcity), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:                "LocalOrdered",
			OrderedStartsBefore: false,
			OrderedSupported:    true,
			MinSize:             2,
			MaxSize:             8,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalOrdered(size), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:             "Priority",
			ScopesSupported:  true,
			MaxTimeSupported: true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalPriorityQueue(size, defaultLocalQueueCapcity), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:                    "LimitedSize",
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			MaxTimeSupported:        true,
			ScopesSupported:         true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalLimitedSize(size, 1024*size), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:                    "LimitedSizeSerializable",
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			MaxTimeSupported:        true,
			ScopesSupported:         true,
			RetrySupported:          true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				q, err := NewLocalLimitedSizeSerializable(size, 1024*size)
				return q, func(ctx context.Context) error { return nil }, err
			},
		},
		{
			Name:             "Shuffled",
			SingleWorker:     true,
			MaxTimeSupported: true,
			ScopesSupported:  true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalShuffled(size, defaultLocalQueueCapcity), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:    "SQSFifo",
			MaxSize: 4,
			Skip:    true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				q, err := NewSQSFifoQueue(randomString(4), size, awsTestCredentialsFromEnv())
				closer := func(ctx context.Context) error { return nil }
				return q, closer, err
			},
		},
	}
}

func MergeQueueTestCases(ctx context.Context, cases ...[]QueueTestCase) <-chan QueueTestCase {
	out := make(chan QueueTestCase)
	go func() {
		defer close(out)
		for _, group := range cases {
			for _, cs := range group {
				select {
				case <-ctx.Done():
					return
				case out <- cs:
				}
			}
		}
	}()
	return out
}

func MongoDBQueueTestCases(client *mongo.Client) []QueueTestCase {
	return []QueueTestCase{
		{
			Name:               "MongoUnordered",
			IsRemote:           true,
			WaitUntilSupported: true,
			MaxTimeSupported:   true,
			ScopesSupported:    true,
			RetrySupported:     true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     defaultMongoDBTestOptions(),
					Client:  client,
				}
				opts.MDB.Format = amboy.BSON2
				q, err := NewMongoDBQueue(ctx, opts)
				if err != nil {
					return nil, nil, err
				}
				rq, ok := q.(remoteQueue)
				if !ok {
					return nil, nil, errors.New("invalid queue constructed")
				}

				closer := func(ctx context.Context) error {
					catcher := grip.NewBasicCatcher()
					d := rq.Driver()
					if d != nil {
						catcher.Add(d.Close(ctx))
					}

					catcher.Add(client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx))

					return catcher.Resolve()
				}

				return q, closer, nil
			},
		},
		{
			Name:               "MongoGroupUnordered",
			IsRemote:           true,
			WaitUntilSupported: true,
			MaxTimeSupported:   true,
			ScopesSupported:    true,
			RetrySupported:     true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     defaultMongoDBTestOptions(),
					Client:  client,
				}
				opts.MDB.Format = amboy.BSON2
				opts.MDB.GroupName = "group." + name
				opts.MDB.UseGroups = true
				q, err := NewMongoDBQueue(ctx, opts)
				if err != nil {
					return nil, nil, err
				}
				rq, ok := q.(remoteQueue)
				if !ok {
					return nil, nil, errors.New("invalid queue constructed")
				}

				closer := func(ctx context.Context) error {
					catcher := grip.NewBasicCatcher()

					d := rq.Driver()
					if d != nil {
						catcher.Add(d.Close(ctx))
					}

					catcher.Add(client.Database(opts.MDB.DB).Collection(addGroupSuffix(name)).Drop(ctx))

					return catcher.Resolve()
				}

				return q, closer, nil
			},
		},
		{
			Name:               "MongoUnorderedMGOBSON",
			IsRemote:           true,
			WaitUntilSupported: true,
			MaxTimeSupported:   true,
			ScopesSupported:    true,
			RetrySupported:     true,
			MaxSize:            32,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     defaultMongoDBTestOptions(),
					Client:  client,
				}
				opts.MDB.Format = amboy.BSON
				q, err := NewMongoDBQueue(ctx, opts)
				if err != nil {
					return nil, nil, err
				}
				rq, ok := q.(remoteQueue)
				if !ok {
					return nil, nil, errors.New("invalid queue constructed")
				}

				closer := func(ctx context.Context) error {
					catcher := grip.NewBasicCatcher()
					d := rq.Driver()
					if d != nil {
						catcher.Add(d.Close(ctx))
					}

					catcher.Add(client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx))

					return catcher.Resolve()
				}

				return q, closer, nil
			},
		},
		{
			Name:               "MongoOrdered",
			IsRemote:           true,
			WaitUntilSupported: true,
			MaxTimeSupported:   true,
			RetrySupported:     true,
			ScopesSupported:    true,
			OrderedSupported:   true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: true,
					MDB:     defaultMongoDBTestOptions(),
					Client:  client,
				}
				opts.MDB.Format = amboy.BSON2
				q, err := NewMongoDBQueue(ctx, opts)
				if err != nil {
					return nil, nil, err
				}
				rq, ok := q.(remoteQueue)
				if !ok {
					return nil, nil, errors.New("invalid queue constructed")
				}

				closer := func(ctx context.Context) error {
					catcher := grip.NewBasicCatcher()
					d := rq.Driver()
					if d != nil {
						catcher.Add(d.Close(ctx))
					}

					catcher.Add(client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx))

					return catcher.Resolve()
				}

				return q, closer, nil
			},
		},
	}

}

func DefaultPoolTestCases() []PoolTestCase {
	return []PoolTestCase{
		{
			Name:    "Default",
			SetPool: func(q amboy.Queue, _ int) error { return nil },
		},
		{
			Name:      "Single",
			SkipMulti: true,
			MinSize:   1,
			MaxSize:   1,
			SetPool: func(q amboy.Queue, _ int) error {
				runner := pool.NewSingle()
				if err := runner.SetQueue(q); err != nil {
					return err
				}

				return q.SetRunner(runner)
			},
		},
		{
			Name:    "Abortable",
			MinSize: 4,
			SetPool: func(q amboy.Queue, size int) error { return q.SetRunner(pool.NewAbortablePool(size, q)) },
		},
		{
			Name:         "RateLimitedSimple",
			MinSize:      4,
			MaxSize:      16,
			RateLimiting: true,
			SetPool: func(q amboy.Queue, size int) error {
				runner, err := pool.NewSimpleRateLimitedWorkers(size, 10*time.Millisecond, q)
				if err != nil {
					return nil
				}

				return q.SetRunner(runner)
			},
		},
		{
			Name:         "RateLimitedAverage",
			MinSize:      4,
			MaxSize:      16,
			RateLimiting: true,
			SkipMulti:    true,
			SkipRemote:   true,
			SetPool: func(q amboy.Queue, size int) error {
				runner, err := pool.NewMovingAverageRateLimitedWorkers(size, size*100, 10*time.Millisecond, q)
				if err != nil {
					return nil
				}

				return q.SetRunner(runner)
			},
		},
	}
}

func DefaultSizeTestCases() []SizeTestCase {
	return []SizeTestCase{
		{Name: "One", Size: 1},
		{Name: "Two", Size: 2},
		{Name: "Four", Size: 4},
		{Name: "Eight", Size: 8},
		{Name: "Sixteen", Size: 16},
		{Name: "ThirtyTwo", Size: 32},
		{Name: "SixtyFour", Size: 64},
	}
}

func TestQueueSmoke(t *testing.T) {
	bctx, bcancel := context.WithCancel(context.Background())
	defer bcancel()

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Connect(bctx))

	defer func() { require.NoError(t, client.Disconnect(bctx)) }()

	for test := range MergeQueueTestCases(bctx, DefaultQueueTestCases(), MongoDBQueueTestCases(client)) {
		if test.Skip {
			continue
		}

		t.Run(test.Name, func(t *testing.T) {
			for _, runner := range DefaultPoolTestCases() {
				if test.IsRemote && runner.SkipRemote {
					continue
				}

				t.Run(runner.Name+"Pool", func(t *testing.T) {
					var (
						testRetryOnce                sync.Once
						testWaitUntilOnce            sync.Once
						testDispatchBeforeOnce       sync.Once
						testMaxTimeOnce              sync.Once
						testScopesOnce               sync.Once
						testApplyScopesOnEnqueueOnce sync.Once
					)

					for _, size := range DefaultSizeTestCases() {
						if test.MaxSize > 0 && size.Size > test.MaxSize {
							continue
						}

						if runner.MinSize > 0 && runner.MinSize > size.Size {
							continue
						}

						if runner.MaxSize > 0 && runner.MaxSize < size.Size {
							continue
						}

						if size.Size > 8 && (runtime.GOOS == "windows" || runtime.GOOS == "darwin" || testing.Short()) {
							continue
						}

						t.Run("MaxSize"+size.Name, func(t *testing.T) {
							if !test.SkipUnordered {
								t.Run("Unordered", func(t *testing.T) {
									UnorderedTest(bctx, t, test, runner, size)
								})
							}
							if test.OrderedSupported {
								t.Run("Ordered", func(t *testing.T) {
									OrderedTest(bctx, t, test, runner, size)
								})
							}
							if test.WaitUntilSupported {
								testWaitUntilOnce.Do(func() {
									t.Run("WaitUntil", func(t *testing.T) {
										WaitUntilTest(bctx, t, test, runner, size)
									})
								})
							}

							if test.DispatchBeforeSupported {
								testDispatchBeforeOnce.Do(func() {
									t.Run("DispatchBefore", func(t *testing.T) {
										DispatchBeforeTest(bctx, t, test, runner, size)
									})
								})
							}
							if test.MaxTimeSupported {
								testMaxTimeOnce.Do(func() {
									t.Run("MaxTime", func(t *testing.T) {
										MaxTimeTest(bctx, t, test, runner, size)
									})
								})
							}

							if test.RetrySupported && size.Size >= 2 {
								testRetryOnce.Do(func() {
									t.Run("Retry", func(t *testing.T) {
										RetryableTest(bctx, t, test, runner, size)
									})
								})
							}

							t.Run("OneExecution", func(t *testing.T) {
								OneExecutionTest(bctx, t, test, runner, size)
							})

							if test.ScopesSupported {
								if test.SingleWorker && (!test.OrderedSupported || test.OrderedStartsBefore) && size.Size >= 4 {
									testScopesOnce.Do(func() {
										t.Run("ScopedLock", func(t *testing.T) {
											ScopedLockTest(bctx, t, test, runner, size)
										})
									})
								}
								if size.Size >= 2 {
									testApplyScopesOnEnqueueOnce.Do(func() {
										t.Run("ApplyScopesOnEnqueue", func(t *testing.T) {
											ApplyScopesOnEnqueueTest(bctx, t, test, runner, size)
										})
									})
								}
							}

							if test.IsRemote && test.MultiSupported && !runner.SkipMulti {
								t.Run("MultiExecution", func(t *testing.T) {
									MultiExecutionTest(bctx, t, test, runner, size)
								})

								if size.Size < 8 {
									t.Run("ManyQueues", func(t *testing.T) {
										ManyQueueTest(bctx, t, test, runner, size)
									})
								}
							}

							t.Run("SaveLockingCheck", func(t *testing.T) {
								if test.OrderedSupported && !test.OrderedStartsBefore {
									t.Skip("test does not support queues where queues don't accept work after dispatching")
								}
								ctx, cancel := context.WithCancel(bctx)
								defer cancel()
								name := newDriverID()

								q, closer, err := test.Constructor(ctx, name, size.Size)
								require.NoError(t, err)
								defer func() { require.NoError(t, closer(ctx)) }()

								require.NoError(t, runner.SetPool(q, size.Size))
								require.NoError(t, err)
								j := amboy.Job(job.NewShellJob("sleep 300", ""))
								j.UpdateTimeInfo(amboy.JobTimeInfo{
									WaitUntil: time.Now().Add(4 * amboy.LockTimeout),
								})
								require.NoError(t, q.Start(ctx))
								require.NoError(t, q.Put(ctx, j))

								require.NoError(t, j.Lock(q.ID(), q.Info().LockTimeout))
								require.NoError(t, q.Save(ctx, j))

								if test.IsRemote {
									// this errors because you can't save if you've double-locked,
									// but only real remote drivers check locks.
									require.NoError(t, j.Lock(q.ID(), q.Info().LockTimeout))
									require.NoError(t, j.Lock(q.ID(), q.Info().LockTimeout))
									require.Error(t, q.Save(ctx, j))
								}

								for i := 0; i < 25; i++ {
									var ok bool
									j, ok = q.Get(ctx, j.ID())
									require.True(t, ok)
									require.NoError(t, j.Lock(q.ID(), q.Info().LockTimeout))
									require.NoError(t, q.Save(ctx, j))
								}

								j, ok := q.Get(ctx, j.ID())
								require.True(t, ok)

								require.NoError(t, j.Error())
								require.NoError(t, q.Complete(ctx, j))
								require.NoError(t, j.Error())
							})
						})
					}
				})
			}
		})
	}
}

func UnorderedTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()

	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()
	require.NoError(t, runner.SetPool(q, size.Size))

	if test.OrderedSupported && !test.OrderedStartsBefore {
		// pass
	} else {
		require.NoError(t, q.Start(ctx))
	}

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false", ""}
	numJobs := size.Size * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < size.Size; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d", name, num)
				j := job.NewShellJob(cmd, "")
				assert.NoError(t, q.Put(ctx, j),
					fmt.Sprintf("with %d workers", num))
				_, ok := q.Get(ctx, j.ID())
				assert.True(t, ok)
			}
		}(i)
	}
	wg.Wait()
	if test.OrderedSupported && !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)

	assert.Equal(t, numJobs, q.Stats(ctx).Total, fmt.Sprintf("with %d workers", size.Size))

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)

	grip.Infof("workers complete for %d worker smoke test", size.Size)
	assert.Equal(t, numJobs, q.Stats(ctx).Completed, fmt.Sprintf("%+v", q.Stats(ctx)))
	for result := range q.Results(ctx) {
		assert.True(t, result.Status().Completed, fmt.Sprintf("with %d workers", size.Size))

		// assert that we had valid time info persisted
		ti := result.TimeInfo()
		assert.NotZero(t, ti.Start)
		assert.NotZero(t, ti.End)
	}

	statCounter := 0
	for info := range q.JobInfo(ctx) {
		statCounter++
		assert.NotEmpty(t, info.ID)
	}
	assert.Equal(t, numJobs, statCounter, fmt.Sprintf("want job info for every job"))

	grip.Infof("completed results check for %d worker smoke test", size.Size)
}

func OrderedTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()

	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()
	require.NoError(t, runner.SetPool(q, size.Size))

	var lastJobName string

	testNames := []string{"amboy", "cusseta", "jasper", "sardis", "dublin"}

	numJobs := size.Size / 2 * len(testNames)

	tempDir, err := ioutil.TempDir("", strings.Join([]string{"amboy-ordered-queue-smoke-test",
		uuid.New().String()}, "-"))
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	if test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}
	for i := 0; i < size.Size/2; i++ {
		for _, name := range testNames {
			fn := filepath.Join(tempDir, fmt.Sprintf("%s.%d", name, i))
			cmd := fmt.Sprintf("echo %s", fn)
			j := job.NewShellJob(cmd, fn)
			if lastJobName != "" {
				require.NoError(t, j.Dependency().AddEdge(lastJobName))
			}
			lastJobName = j.ID()

			require.NoError(t, q.Put(ctx, j))
		}
	}

	if !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	require.Equal(t, numJobs, q.Stats(ctx).Total, fmt.Sprintf("with %d workers", size.Size))
	amboy.WaitInterval(ctx, q, 50*time.Millisecond)
	require.Equal(t, numJobs, q.Stats(ctx).Completed, fmt.Sprintf("%+v", q.Stats(ctx)))
	for result := range q.Results(ctx) {
		require.True(t, result.Status().Completed, fmt.Sprintf("with %d workers", size.Size))
	}

	statCounter := 0
	for info := range q.JobInfo(ctx) {
		statCounter++
		require.NotEmpty(t, info.ID)
	}
	require.Equal(t, statCounter, numJobs)
}

func WaitUntilTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()
	require.NoError(t, runner.SetPool(q, size.Size))

	require.NoError(t, q.Start(ctx))

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false"}

	sz := size.Size
	if sz > 16 {
		sz = 16
	} else if sz < 2 {
		sz = 2
	}
	numJobs := sz * len(testNames)
	wg := &sync.WaitGroup{}

	for i := 0; i < sz; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d.default", name, num)
				j := job.NewShellJob(cmd, "")
				ti := j.TimeInfo()
				require.Zero(t, ti.WaitUntil)
				require.NoError(t, q.Put(ctx, j), fmt.Sprintf("(a) with %d workers", num))
				_, ok := q.Get(ctx, j.ID())
				require.True(t, ok)

				cmd = fmt.Sprintf("echo %s.%d.waiter", name, num)
				j2 := job.NewShellJob(cmd, "")
				j2.UpdateTimeInfo(amboy.JobTimeInfo{
					WaitUntil: time.Now().Add(time.Hour),
				})
				ti2 := j2.TimeInfo()
				require.NotZero(t, ti2.WaitUntil)
				require.NoError(t, q.Put(ctx, j2), fmt.Sprintf("(b) with %d workers", num))
				_, ok = q.Get(ctx, j2.ID())
				require.True(t, ok)
			}
		}(i)
	}
	wg.Wait()
	// waitC for things to finish
	const (
		interval = 100 * time.Millisecond
		maxTime  = 3 * time.Second
	)
	var dur time.Duration
	timer := time.NewTimer(interval)
	defer timer.Stop()
waitLoop:
	for {
		select {
		case <-ctx.Done():
			break waitLoop
		case <-timer.C:
			dur += interval
			stat := q.Stats(ctx)
			if stat.Completed >= numJobs {
				break waitLoop
			}

			if dur >= maxTime {
				break waitLoop
			}

			timer.Reset(interval)
		}
	}

	stats := q.Stats(ctx)
	require.Equal(t, numJobs*2, stats.Total, "%+v", stats)
	assert.Equal(t, numJobs, stats.Completed)

	completed := 0
	for result := range q.Results(ctx) {
		status := result.Status()
		ti := result.TimeInfo()

		if status.Completed {
			completed++
			require.True(t, ti.WaitUntil.IsZero(), "val=%s id=%s", ti.WaitUntil, result.ID())
		} else {
			require.False(t, ti.WaitUntil.IsZero(), "val=%s id=%s", ti.WaitUntil, result.ID())
		}
	}

	assert.Equal(t, numJobs, completed)
}

func DispatchBeforeTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()
	require.NoError(t, runner.SetPool(q, size.Size))

	require.NoError(t, q.Start(ctx))

	for i := 0; i < 2*size.Size; i++ {
		j := job.NewShellJob("ls", "")
		ti := j.TimeInfo()

		if i%2 == 0 {
			ti.DispatchBy = time.Now().Add(time.Second)
		} else {
			ti.DispatchBy = time.Now().Add(-time.Second)
		}
		j.UpdateTimeInfo(ti)
		require.NoError(t, q.Put(ctx, j))
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

waitLoop:
	for {
		select {
		case <-ctx.Done():
			break waitLoop
		case <-ticker.C:
			stat := q.Stats(ctx)
			if stat.Completed == size.Size {
				break waitLoop
			}
		}
	}

	stats := q.Stats(ctx)
	assert.Equal(t, 2*size.Size, stats.Total)
	assert.Equal(t, size.Size, stats.Completed)
}

func MaxTimeTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 10*time.Second)
	defer cancel()
	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, closer(ctx))
	}()

	require.NoError(t, q.Start(ctx))

	sleepTime := 5 * time.Second
	var jobIDs []string
	for i := 0; i < size.Size; i++ {
		j := newSleepJob()
		j.Sleep = sleepTime
		j.UpdateTimeInfo(amboy.JobTimeInfo{
			MaxTime: time.Millisecond,
		})
		require.NoError(t, q.Put(ctx, j))
		jobIDs = append(jobIDs, j.ID())
	}

	require.True(t, amboy.WaitInterval(ctx, q, 100*time.Millisecond))
	for _, jobID := range jobIDs {
		j, ok := q.Get(ctx, jobID)
		require.True(t, ok, "job %s not in queue", jobID)
		assert.True(t, j.TimeInfo().End.Sub(j.TimeInfo().Start) < sleepTime, "job should have run for less than specified max time")
	}
}

func OneExecutionTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	if test.Name == "LocalOrdered" {
		t.Skip("topological sort deadlocks")
	}
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))

	defer func() { require.NoError(t, closer(ctx)) }()

	mockJobCounters.Reset()
	count := 40

	if !test.OrderedSupported || test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	for i := 0; i < count; i++ {
		j := newMockJob()
		jobID := fmt.Sprintf("%d.%d.mock.single-exec", i, job.GetNumber())
		j.SetID(jobID)
		assert.NoError(t, q.Put(ctx, j))
	}

	if test.OrderedSupported && !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)
	assert.Equal(t, count, mockJobCounters.Count())
}

func MultiExecutionTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()
	name := newDriverID()
	qOne, closerOne, err := test.Constructor(ctx, name, size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closerOne(ctx)) }()
	qTwo, closerTwo, err := test.Constructor(ctx, name, size.Size)
	defer func() { require.NoError(t, closerTwo(ctx)) }()
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(qOne, size.Size))
	require.NoError(t, runner.SetPool(qTwo, size.Size))

	assert.NoError(t, qOne.Start(ctx))
	assert.NoError(t, qTwo.Start(ctx))

	num := 200
	adderProcs := 4

	wg := &sync.WaitGroup{}
	for o := 0; o < adderProcs; o++ {
		wg.Add(1)
		go func(o int) {
			defer wg.Done()
			// add a bunch of jobs: half to one queue and half to the other.
			for i := 0; i < num; i++ {
				cmd := fmt.Sprintf("echo %d.%d", o, i)
				j := job.NewShellJob(cmd, "")
				if i%2 == 0 {
					assert.NoError(t, qOne.Put(ctx, j))
				} else {
					assert.NoError(t, qTwo.Put(ctx, j))
				}

			}
		}(o)
	}
	wg.Wait()

	num = num * adderProcs

	grip.Info("added jobs to queues")

	// wait for all jobs to complete.
	amboy.WaitInterval(ctx, qOne, 100*time.Millisecond)
	amboy.WaitInterval(ctx, qTwo, 100*time.Millisecond)

	// check that both queues see all jobs
	statsOne := qOne.Stats(ctx)
	statsTwo := qTwo.Stats(ctx)

	var shouldExit bool
	if !assert.Equal(t, num, statsOne.Total, "ONE: %+v", statsOne) {
		shouldExit = true
	}
	if !assert.Equal(t, num, statsTwo.Total, "TWO: %+v", statsTwo) {
		shouldExit = true
	}
	if shouldExit {
		return
	}

	// check that all the results in the queues are are completed,
	// and unique
	firstCount := 0
	results := make(map[string]struct{})
	for result := range qOne.Results(ctx) {
		firstCount++
		assert.True(t, result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	secondCount := 0
	// make sure that all of the results in the second queue match
	// the results in the first queue.
	for result := range qTwo.Results(ctx) {
		secondCount++
		assert.True(t, result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	assert.Equal(t, firstCount, secondCount)
	assert.Equal(t, len(results), firstCount)
}

func ManyQueueTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()

	driverID := newDriverID()
	sz := size.Size
	if sz > 8 {
		sz = 8
	} else if sz < 2 {
		sz = 2
	}

	queues := []remoteQueue{}
	for i := 0; i < sz; i++ {
		q, closer, err := test.Constructor(ctx, driverID, size.Size)
		require.NoError(t, err)
		defer func() { require.NoError(t, closer(ctx)) }()
		queue := q.(remoteQueue)

		require.NoError(t, q.Start(ctx))
		queues = append(queues, queue)
	}

	const (
		inside  = 15
		outside = 10
	)

	mockJobCounters.Reset()
	wg := &sync.WaitGroup{}
	for i := 0; i < size.Size; i++ {
		for ii := 0; ii < outside; ii++ {
			wg.Add(1)
			go func(f, s int) {
				defer wg.Done()
				for iii := 0; iii < inside; iii++ {
					j := newMockJob()
					j.SetID(fmt.Sprintf("%d-%d-%d-%d", f, s, iii, job.GetNumber()))
					assert.NoError(t, queues[0].Put(ctx, j))
				}
			}(i, ii)
		}
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	for _, q := range queues {
		amboy.WaitInterval(ctx, q, 20*time.Millisecond)
	}

	assert.Equal(t, size.Size*inside*outside, mockJobCounters.Count())
}

func ScopedLockTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, time.Minute)
	defer cancel()
	q, closer, err := test.Constructor(ctx, newDriverID(), 2*size.Size)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()
	require.NoError(t, runner.SetPool(q, size.Size*3))

	require.NoError(t, q.Start(ctx))

	for i := 0; i < 2*size.Size; i++ {
		j := newSleepJob()
		if i%2 == 0 {
			j.SetScopes([]string{"a"})
			j.Sleep = time.Hour
		}
		require.NoError(t, q.Put(ctx, j))
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

waitLoop:
	for {
		select {
		case <-ctx.Done():
			break waitLoop
		case <-ticker.C:
			stat := q.Stats(ctx)
			if stat.Completed >= size.Size {
				break waitLoop
			}
		}
	}

	time.Sleep(50 * time.Millisecond)
	stats := q.Stats(ctx)
	assert.Equal(t, 2*size.Size, stats.Total)
	assert.Equal(t, size.Size, stats.Completed)
}

func ApplyScopesOnEnqueueTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, q amboy.Queue){
		"PutJobAppliesScopeAndPreservesSettings": func(ctx context.Context, t *testing.T, q amboy.Queue) {
			j := newSleepJob()
			j.Sleep = 10 * time.Millisecond
			j.SetScopes([]string{"scope"})
			j.SetShouldApplyScopesOnEnqueue(true)

			require.NoError(t, q.Put(ctx, j))
			fetchedJob, ok := q.Get(ctx, j.ID())
			require.True(t, ok)
			assert.EqualValues(t, j.Scopes(), fetchedJob.Scopes())
			assert.True(t, fetchedJob.ShouldApplyScopesOnEnqueue())
		},
		"PutJobFollowedBySaveSucceeds": func(ctx context.Context, t *testing.T, q amboy.Queue) {
			j := newSleepJob()
			j.Sleep = 10 * time.Millisecond
			j.SetScopes([]string{"scope"})
			j.SetShouldApplyScopesOnEnqueue(true)
			require.NoError(t, q.Put(ctx, j))
			require.NoError(t, q.Save(ctx, j))
		},
		"PutJobPreventsEnqueueingDuplicateScopeUntilJobCompletes": func(ctx context.Context, t *testing.T, q amboy.Queue) {
			j1 := newSleepJob()
			j1.Sleep = 10 * time.Millisecond
			j1.SetScopes([]string{"scope"})
			j1.SetShouldApplyScopesOnEnqueue(true)

			j2 := newSleepJob()
			j2.Sleep = 10 * time.Millisecond
			j2.SetScopes([]string{"scope"})
			j2.SetShouldApplyScopesOnEnqueue(true)

			require.NoError(t, q.Put(ctx, j1))
			require.Error(t, q.Put(ctx, j2))

			require.True(t, amboy.WaitInterval(ctx, q, 10*time.Millisecond))

			require.NoError(t, q.Put(ctx, j2))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(bctx, time.Minute)
			defer cancel()

			q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, closer(ctx))
			}()
			require.NoError(t, runner.SetPool(q, size.Size))

			require.NoError(t, q.Start(ctx))

			testCase(ctx, t, q)
		})
	}
}

func RetryableTest(bctx context.Context, t *testing.T, test QueueTestCase, runner PoolTestCase, size SizeTestCase) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, rh amboy.RetryHandler, rq amboy.RetryableQueue){
		"JobRetriesOnce": func(ctx context.Context, t *testing.T, rh amboy.RetryHandler, rq amboy.RetryableQueue) {
			j := newMockRetryableJob("id")
			j.NumTimesToRetry = 1

			require.NoError(t, rq.Put(ctx, j))
			require.True(t, amboy.WaitInterval(ctx, rq, 100*time.Millisecond))

			jobReenqueued := make(chan struct{})
			go func() {
				defer close(jobReenqueued)
				for {
					if ctx.Err() != nil {
						return
					}
					if rq.Stats(ctx).IsComplete() {
						return
					}
				}
			}()
			select {
			case <-ctx.Done():
				require.FailNow(t, ctx.Err().Error())
			case <-jobReenqueued:
				assert.True(t, rq.Stats(ctx).Total > 1)
				require.True(t, amboy.WaitInterval(ctx, rq, 100*time.Millisecond))
				var foundFirstAttempt, foundSecondAttempt bool
				for completed := range rq.Results(ctx) {
					assert.False(t, completed.RetryInfo().ShouldRetry())
					if completed.RetryInfo().CurrentAttempt == 0 {
						foundFirstAttempt = true
					}
					if completed.RetryInfo().CurrentAttempt == 1 {
						foundSecondAttempt = true
					}
				}
				assert.True(t, foundFirstAttempt, "first job attempt should have completed")
				assert.True(t, foundSecondAttempt, "second job attempt should have completed")
			}
		},
		"ScopedJobRetriesOnceThenAllowsLaterJobToTakeScope": func(ctx context.Context, t *testing.T, rh amboy.RetryHandler, rq amboy.RetryableQueue) {
			j := newMockRetryableJob("id")
			j.NumTimesToRetry = 1
			j.SetShouldApplyScopesOnEnqueue(true)
			scopes := []string{"scope"}
			j.SetScopes(scopes)

			require.NoError(t, rq.Put(ctx, j))

			jobAfterRetry := newMockRetryableJob("id1")
			jobAfterRetry.SetScopes(scopes)
			require.True(t, amboy.WaitInterval(ctx, rq, 100*time.Millisecond))

			require.NoError(t, rq.Put(ctx, jobAfterRetry))
			require.True(t, amboy.WaitInterval(ctx, rq, 100*time.Millisecond))

			assert.Equal(t, 3, rq.Stats(ctx).Completed)
			var foundFirstAttempt, foundSecondAttempt bool
			for completed := range rq.Results(ctx) {
				assert.False(t, completed.RetryInfo().ShouldRetry())
				if completed.RetryInfo().CurrentAttempt == 0 {
					foundFirstAttempt = true
				}
				if completed.RetryInfo().CurrentAttempt == 1 {
					foundSecondAttempt = true
				}
			}
			assert.True(t, foundFirstAttempt, "first job attempt should have completed")
			assert.True(t, foundSecondAttempt, "second job attempt should have completed")
		},
		"StaleRetryingJobsAreDetectedAndRetried": func(ctx context.Context, t *testing.T, rh amboy.RetryHandler, rq amboy.RetryableQueue) {
			j := newMockRetryableJob("id")
			j.SetStatus(amboy.JobStatusInfo{
				Completed:        true,
				ModificationTime: time.Now().Add(-100 * rq.Info().LockTimeout),
			})
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				NeedsRetry: utility.TruePtr(),
			})

			require.NoError(t, rq.Put(ctx, j))
			jobsDone := make(chan struct{})
			go func() {
				defer close(jobsDone)
				for {
					if ctx.Err() != nil {
						return
					}
					if rq.Stats(ctx).IsComplete() {
						return
					}
				}
			}()

			select {
			case <-ctx.Done():
				require.FailNow(t, "context was done before stale retrying job could be handled")
			case <-jobsDone:
				assert.Equal(t, 2, rq.Stats(ctx).Total)
				assert.Equal(t, 2, rq.Stats(ctx).Completed)

				rj0, err := rq.GetAttempt(ctx, j.ID(), 0)
				require.NoError(t, err)
				assert.True(t, rj0.RetryInfo().Retryable)
				assert.False(t, rj0.RetryInfo().NeedsRetry)

				rj1, err := rq.GetAttempt(ctx, j.ID(), 1)
				require.NoError(t, err)
				assert.True(t, rj1.RetryInfo().Retryable)
				assert.False(t, rj1.RetryInfo().NeedsRetry)
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(bctx, time.Minute)
			defer cancel()

			q, closer, err := test.Constructor(ctx, newDriverID(), size.Size)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, closer(ctx))
			}()

			rq, ok := q.(amboy.RetryableQueue)
			require.True(t, ok, "queue is not retryable")

			require.NoError(t, runner.SetPool(rq, size.Size))

			require.NoError(t, rq.Start(ctx))

			testCase(ctx, t, rq.RetryHandler(), rq)
		})
	}

}
