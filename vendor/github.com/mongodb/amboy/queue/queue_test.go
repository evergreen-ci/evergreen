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

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultLocalQueueCapcity = 10000

func init() {
	grip.SetName("amboy.queue.tests")
	grip.Error(grip.SetSender(send.MakeNative()))

	if !testing.Verbose() {
		lvl := grip.GetSender().Level()
		lvl.Threshold = level.Error
		_ = grip.GetSender().SetLevel(lvl)
	}

	job.RegisterDefaultJobs()
}

func newDriverID() string { return strings.Replace(uuid.NewV4().String(), "-", ".", -1) }

type TestCloser func(context.Context) error

type QueueTestCase struct {
	Name                    string
	Constructor             func(context.Context, string, int) (amboy.Queue, TestCloser, error)
	MinSize                 int
	MaxSize                 int
	MultiSupported          bool
	OrderedSupported        bool
	OrderedStartsBefore     bool
	WaitUntilSupported      bool
	DispatchBeforeSupported bool
	SkipUnordered           bool
	IsRemote                bool
	Skip                    bool
}

type PoolTestCase struct {
	Name       string
	SetPool    func(amboy.Queue, int) error
	SkipRemote bool
	SkipMulti  bool
	MinSize    int
	MaxSize    int
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
			DispatchBeforeSupported: true,
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
			Name: "Priority",
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalPriorityQueue(size, defaultLocalQueueCapcity), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:                    "LimitedSize",
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewLocalLimitedSize(size, 1024*size), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name: "Shuffled",
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				return NewShuffledLocal(size, defaultLocalQueueCapcity), func(ctx context.Context) error { return nil }, nil
			},
		},
		{
			Name:    "SQSFifo",
			MaxSize: 4,
			Skip:    true,
			Constructor: func(ctx context.Context, _ string, size int) (amboy.Queue, TestCloser, error) {
				q, err := NewSQSFifoQueue(randomString(4), size)
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
			Name:     "MongoUnordered",
			IsRemote: true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     DefaultMongoDBOptions(),
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
					d := rq.Driver()
					if d != nil {
						d.Close()
					}

					return client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return q, closer, nil
			},
		},
		{
			Name:     "MongoGroupUnordered",
			IsRemote: true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     DefaultMongoDBOptions(),
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
					d := rq.Driver()
					if d != nil {
						d.Close()
					}

					return client.Database(opts.MDB.DB).Collection(addGroupSufix(name)).Drop(ctx)
				}

				return q, closer, nil
			},
		},
		{
			Name:     "MongoUnorderedMGOBSON",
			IsRemote: true,
			MaxSize:  32,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: false,
					MDB:     DefaultMongoDBOptions(),
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
					d := rq.Driver()
					if d != nil {
						d.Close()
					}

					return client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return q, closer, nil
			},
		},
		{
			Name:     "MongoOrdered",
			IsRemote: true,
			Constructor: func(ctx context.Context, name string, size int) (amboy.Queue, TestCloser, error) {
				opts := MongoDBQueueCreationOptions{
					Size:    size,
					Name:    name,
					Ordered: true,
					MDB:     DefaultMongoDBOptions(),
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
					d := rq.Driver()
					if d != nil {
						d.Close()
					}

					return client.Database(opts.MDB.DB).Collection(addJobsSuffix(name)).Drop(ctx)
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
			Name:    "RateLimitedSimple",
			MinSize: 4,
			MaxSize: 16,
			SetPool: func(q amboy.Queue, size int) error {
				runner, err := pool.NewSimpleRateLimitedWorkers(size, 10*time.Millisecond, q)
				if err != nil {
					return nil
				}

				return q.SetRunner(runner)
			},
		},
		{
			Name:       "RateLimitedAverage",
			MinSize:    4,
			MaxSize:    16,
			SkipMulti:  true,
			SkipRemote: true,
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

						t.Run(size.Name, func(t *testing.T) {
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
								t.Run("WaitUntil", func(t *testing.T) {
									WaitUntilTest(bctx, t, test, runner, size)
								})
							}

							if test.DispatchBeforeSupported {
								t.Run("DispatchBefore", func(t *testing.T) {
									DispatchBeforeTest(bctx, t, test, runner, size)
								})
							}

							t.Run("OneExecution", func(t *testing.T) {
								OneExecutionTest(bctx, t, test, runner, size)
							})

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

								require.NoError(t, j.Lock(q.ID()))
								require.NoError(t, q.Save(ctx, j))

								if test.IsRemote {
									// this errors because you can't save if you've double-locked,
									// but only real remote drivers check locks.
									require.NoError(t, j.Lock(q.ID()))
									require.NoError(t, j.Lock(q.ID()))
									require.Error(t, q.Save(ctx, j))
								}

								for i := 0; i < 25; i++ {
									j, ok := q.Get(ctx, j.ID())
									require.True(t, ok)
									require.NoError(t, j.Lock(q.ID()))
									require.NoError(t, q.Save(ctx, j))
								}

								j, ok := q.Get(ctx, j.ID())
								require.True(t, ok)

								require.NoError(t, j.Error())
								q.Complete(ctx, j)
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
	for stat := range q.JobStats(ctx) {
		statCounter++
		assert.True(t, stat.ID != "")
	}
	assert.Equal(t, numJobs, statCounter, fmt.Sprintf("want jobStats for every job"))

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
		uuid.NewV4().String()}, "-"))
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
	for stat := range q.JobStats(ctx) {
		statCounter++
		require.True(t, stat.ID != "")
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
