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
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2"
)

func init() {
	grip.SetName("amboy.queue.tests")
	grip.Error(grip.SetSender(send.MakeNative()))

	lvl := grip.GetSender().Level()
	lvl.Threshold = level.Notice
	_ = grip.GetSender().SetLevel(lvl)

	job.RegisterDefaultJobs()
}

const defaultLocalQueueCapcity = 10000

////////////////////////////////////////////////////////////////////////////////
//
// Generic smoke/integration tests for queues.
//
////////////////////////////////////////////////////////////////////////////////

func runUnorderedSmokeTest(ctx context.Context, q amboy.Queue, size int, assert *assert.Assertions) {
	numJobs := populateUnorderedSmokeTest(ctx, q, size, assert)
	assert.Equal(numJobs, q.Stats().Total, fmt.Sprintf("with %d workers", size))

	amboy.WaitCtxInterval(ctx, q, 100*time.Millisecond)

	grip.Infof("workers complete for %d worker smoke test", size)
	assert.Equal(numJobs, q.Stats().Completed, fmt.Sprintf("%+v", q.Stats()))
	for result := range q.Results(ctx) {
		assert.True(result.Status().Completed, fmt.Sprintf("with %d workers", size))

		// assert that we had valid time info persisted
		ti := result.TimeInfo()
		assert.NotZero(ti.Start)
		assert.NotZero(ti.End)
	}

	statCounter := 0
	for stat := range q.JobStats(ctx) {
		statCounter++
		assert.True(stat.ID != "")
	}
	assert.Equal(numJobs, statCounter, fmt.Sprintf("want jobStats for every job"))

	grip.Infof("completed results check for %d worker smoke test", size)
}

func populateUnorderedSmokeTest(ctx context.Context, q amboy.Queue, size int, assert *assert.Assertions) int {
	if err := q.Start(ctx); !assert.NoError(err) {
		return 0
	}

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false", ""}
	numJobs := size * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < size; i++ {
		wg.Add(1)
		go func(num int) {
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d", name, num)
				j := job.NewShellJob(cmd, "")
				assert.NoError(q.Put(j),
					fmt.Sprintf("with %d workers", num))
				_, ok := q.Get(j.ID())
				assert.True(ok)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	amboy.WaitCtxInterval(ctx, q, 100*time.Millisecond)

	return numJobs
}

// Simple does not check numJobs against Stats values in the case of Queue update delay
func runSimpleUnorderedSmokeTest(ctx context.Context, q amboy.Queue, size int,
	assert *assert.Assertions) {
	if err := q.Start(ctx); !assert.NoError(err) {
		return
	}

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false", ""}
	numJobs := size * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < size; i++ {
		wg.Add(1)
		go func(num int) {
			for _, name := range testNames {
				j := newMockJob()
				j.SetID(fmt.Sprintf("%s.%d", name, num))
				assert.NoError(q.Put(j),
					fmt.Sprintf("with %d workers", num))
				_, ok := q.Get(j.ID())
				assert.True(ok)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	amboy.WaitCtxInterval(ctx, q, time.Minute)

	grip.Infof("workers complete for %d worker smoke test", size)
	assert.True(q.Stats().Total <= numJobs)
	for result := range q.Results(ctx) {
		assert.True(result.Status().Completed, fmt.Sprintf("with %d workers", size))
		// assert that we had valid time info persisted
		ti := result.TimeInfo()
		assert.NotZero(ti.Start)
		assert.NotZero(ti.End)
	}

	stats := q.Stats()
	grip.Debug(stats)

	statCounter := 0
	for stat := range q.JobStats(ctx) {
		statCounter++
		assert.True(stat.ID != "")
	}
	assert.Equal(numJobs, statCounter, fmt.Sprintf("want jobStats for every job"))
	grip.Infof("completed results check for %d worker smoke test", size)
}

func runMultiQueueSingleBackEndSmokeTest(ctx context.Context, qOne, qTwo amboy.Queue, shared bool, assert *assert.Assertions) {
	assert.NoError(qOne.Start(ctx))
	assert.NoError(qTwo.Start(ctx))

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
					assert.NoError(qOne.Put(j))
					assert.Error(qOne.Put(j))
					continue
				}
				assert.NoError(qTwo.Put(j))
			}
		}(o)
	}
	wg.Wait()

	num = num * adderProcs

	grip.Info("added jobs to queues")

	// check that both queues see all jobs
	statsOne := qOne.Stats()
	statsTwo := qTwo.Stats()

	if shared {
		assert.Equal(statsOne.Total, num)
		assert.Equal(statsTwo.Total, num)
	} else {
		assert.Equal(statsOne.Total+statsTwo.Total, num)
	}

	grip.Infof("before wait statsOne: %+v", statsOne)
	grip.Infof("before wait statsTwo: %+v", statsTwo)

	// wait for all jobs to complete.
	amboy.WaitCtxInterval(ctx, qOne, 100*time.Millisecond)
	amboy.WaitCtxInterval(ctx, qTwo, 100*time.Millisecond)

	grip.Infof("after wait statsOne: %+v", qOne.Stats())
	grip.Infof("after wait statsTwo: %+v", qTwo.Stats())

	// check that all the results in the queues are are completed,
	// and unique
	firstCount := 0
	results := make(map[string]struct{})
	for result := range qOne.Results(ctx) {
		firstCount++
		assert.True(result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	secondCount := 0
	// make sure that all of the results in the second queue match
	// the results in the first queue.
	for result := range qTwo.Results(ctx) {
		secondCount++
		assert.True(result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	if !shared {
		assert.Equal(firstCount+secondCount, len(results))
	}
}

func runOrderedSmokeTest(ctx context.Context, q amboy.Queue, size int, startBefore bool, assert *assert.Assertions) {
	var lastJobName string

	testNames := []string{"amboy", "cusseta", "jasper", "sardis", "dublin"}

	numJobs := size / 2 * len(testNames)

	tempDir, err := ioutil.TempDir("", strings.Join([]string{"amboy-ordered-queue-smoke-test",
		uuid.NewV4().String()}, "-"))
	assert.NoError(err)
	defer os.RemoveAll(tempDir)

	if startBefore {
		if err := q.Start(ctx); !assert.NoError(err) {
			return
		}
	}
	for i := 0; i < size/2; i++ {
		for _, name := range testNames {
			fn := filepath.Join(tempDir, fmt.Sprintf("%s.%d", name, i))
			cmd := fmt.Sprintf("echo %s", fn)
			j := job.NewShellJob(cmd, fn)
			if lastJobName != "" {
				assert.NoError(j.Dependency().AddEdge(lastJobName))
			}
			lastJobName = j.ID()

			assert.NoError(q.Put(j))
		}
	}

	if !startBefore {
		if err := q.Start(ctx); !assert.NoError(err) {
			return
		}
	}

	assert.Equal(numJobs, q.Stats().Total, fmt.Sprintf("with %d workers", size))
	amboy.WaitCtxInterval(ctx, q, 50*time.Millisecond)
	assert.Equal(numJobs, q.Stats().Completed, fmt.Sprintf("%+v", q.Stats()))
	for result := range q.Results(ctx) {
		assert.True(result.Status().Completed, fmt.Sprintf("with %d workers", size))
	}

	statCounter := 0
	for stat := range q.JobStats(ctx) {
		statCounter++
		assert.True(stat.ID != "")
	}
	assert.Equal(statCounter, numJobs)

}

func runWaitUntilSmokeTest(ctx context.Context, q amboy.Queue, size int, assert *assert.Assertions) {
	if err := q.Start(ctx); !assert.NoError(err) {
		return
	}

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false", ""}
	numJobs := size * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < size; i++ {
		wg.Add(1)
		go func(num int) {
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d.a", name, num)
				j := job.NewShellJob(cmd, "")
				ti := j.TimeInfo()
				assert.Zero(ti.WaitUntil)
				assert.NoError(q.Put(j),
					fmt.Sprintf("(a) with %d workers", num))
				_, ok := q.Get(j.ID())
				assert.True(ok)

				cmd = fmt.Sprintf("echo %s.%d.b", name, num)
				j2 := job.NewShellJob(cmd, "")
				j2.UpdateTimeInfo(amboy.JobTimeInfo{
					WaitUntil: time.Now().Add(time.Hour),
				})
				ti2 := j2.TimeInfo()
				assert.NotZero(ti2.WaitUntil)
				assert.NoError(q.Put(j2),
					fmt.Sprintf("(b) with %d workers", num))
				_, ok = q.Get(j2.ID())
				assert.True(ok)
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	assert.Equal(numJobs*2, q.Stats().Total, fmt.Sprintf("with %d workers", size))

	// wait for things to finish
	time.Sleep(2 * time.Second)

	completed := 0
	for result := range q.Results(ctx) {
		status := result.Status()
		ti := result.TimeInfo()

		if status.Completed || status.InProgress {
			completed++
			assert.Zero(ti.WaitUntil)
			continue
		}

		assert.NotZero(ti.WaitUntil)
	}
}

//////////////////////////////////////////////////////////////////////
//
// Integration tests with different queue and driver implementations
//
//////////////////////////////////////////////////////////////////////

func TestUnorderedSingleThreadedLocalPool(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewLocalUnordered(1)
	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestUnorderedSingleThreadedSingleRunner(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewLocalUnordered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestSmokeUnorderedWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithCancel(context.Background())

		q := NewLocalUnordered(poolSize)
		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeRateLimitedSimplePoolUnorderedQueue(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16} {
		ctx, cancel := context.WithCancel(context.Background())

		q := NewLocalUnordered(poolSize)
		runner, err := pool.NewSimpleRateLimitedWorkers(poolSize, 10*time.Millisecond, q)
		assert.NoError(err)
		assert.NotNil(runner)
		assert.NoError(q.SetRunner(runner))

		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeRateLimitedAveragePoolUnorderedQueue(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16} {
		ctx, cancel := context.WithCancel(context.Background())

		q := NewLocalUnordered(poolSize)
		runner, err := pool.NewMovingAverageRateLimitedWorkers(poolSize, 100, 10*time.Second, q)
		assert.NoError(err)
		assert.NotNil(runner)
		assert.NoError(q.SetRunner(runner))

		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeRemoteUnorderedWorkerSingleThreadedWithInternalDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	q := NewRemoteUnordered(1)
	d := NewInternalDriver()
	defer d.Close()
	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestSmokeRemoteUnorderedWorkerPoolsWithInternalDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q := NewRemoteUnordered(poolSize)
		d := NewInternalDriver()
		assert.NoError(q.SetDriver(d))
		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
		d.Close()
	}
}

func TestSmokeRemoteUnorderedSingleThreadedWithMgoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewRemoteUnordered(1)
	d := NewMgoDriver(name, opts).(*mgoDriver)

	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)
	cancel()
	d.Close()
	grip.Error(cleanupMgo(opts.DB, name, d.session.Clone()))
}

func TestSmokeRemoteUnorderedSingleThreadedWithMongoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewRemoteUnordered(1)
	d := NewMongoDriver(name, opts).(*mongoDriver)

	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)

	grip.Error(cleanupMongo(ctx, opts.DB, name, d.client))

	d.Close()

	cancel()
}

func TestSmokeRemoteUnorderedSingleRunnerWithMgoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewRemoteUnordered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewMgoDriver(name, opts).(*mgoDriver)
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)
	cancel()
	d.Close()
	grip.Error(cleanupMgo(opts.DB, name, d.session.Clone()))
}

func TestSmokeRemoteUnorderedSingleRunnerWithMongoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewRemoteUnordered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewMongoDriver(name, opts).(*mongoDriver)
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)
	d.Close()
	grip.Error(cleanupMongo(ctx, opts.DB, name, d.client))
	cancel()
}

func TestSmokeRemoteUnorderedWorkerPoolsWithMgoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		start := time.Now()
		grip.Infof("running mongodb queue smoke test with %d jobs", poolSize)
		q := NewRemoteUnordered(poolSize)
		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

		ctx, cancel := context.WithCancel(baseCtx)
		d := NewMgoDriver(name, opts).(*mgoDriver)
		assert.NoError(q.SetDriver(d))

		runUnorderedSmokeTest(ctx, q, poolSize, assert)
		cancel()
		d.Close()

		grip.Infof("test with %d jobs, duration = %s", poolSize, time.Since(start))
		err := cleanupMgo(opts.DB, name, d.session.Clone())
		grip.AlertWhenf(err != nil, "encountered error cleaning up %s: %+v", name, err)
	}
}

func TestSmokeRemoteUnorderedWorkerPoolsWithMongoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		start := time.Now()
		grip.Infof("running mongodb queue smoke test with %d jobs", poolSize)
		q := NewRemoteUnordered(poolSize)
		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

		ctx, cancel := context.WithCancel(baseCtx)
		d := NewMongoDriver(name, opts).(*mongoDriver)
		assert.NoError(q.SetDriver(d))

		runUnorderedSmokeTest(ctx, q, poolSize, assert)
		cancel()
		d.Close()

		grip.Infof("test with %d jobs, duration = %s", poolSize, time.Since(start))
		err := cleanupMongo(ctx, opts.DB, name, d.client)
		grip.AlertWhenf(err != nil, "encountered error cleaning up %s: %+v", name, err)
	}
}

func TestSmokePriorityQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	q := NewLocalPriorityQueue(1, defaultLocalQueueCapcity)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))

	assert.NoError(q.SetRunner(runner))
	assert.Equal(runner, q.Runner())

	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestSmokePriorityQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 6, 7, 16, 32, 64} {
		grip.Infoln("testing priority queue for:", poolSize)
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q := NewLocalPriorityQueue(poolSize, defaultLocalQueueCapcity)
		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeSQSFifoQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	q, err := NewSQSFifoQueue(randomString(4), 1)
	assert.NoError(err)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))

	assert.NoError(q.SetRunner(runner))
	assert.Equal(runner, q.Runner())

	runSimpleUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestSmokeSQSFifoQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4} {
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q, err := NewSQSFifoQueue(randomString(4), poolSize)
		assert.NoError(err)
		runSimpleUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeSQSFifoQueueWithAbortablePools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()
	for _, poolSize := range []int{2, 4} {
		grip.Infoln("testing priority queue for:", poolSize)
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q, err := NewSQSFifoQueue(randomString(4), 1)
		assert.NoError(err)
		q.SetRunner(pool.NewAbortablePool(poolSize, q))
		runSimpleUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeSQSFifoQueueWithRateLimitingPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4} {
		grip.Infoln("testing priority queue for:", poolSize)
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q, err := NewSQSFifoQueue(randomString(4), 1)
		assert.NoError(err)
		runner, _ := pool.NewSimpleRateLimitedWorkers(poolSize, time.Millisecond, q)
		q.SetRunner(runner)
		runSimpleUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokePriorityDriverWithRemoteQueueSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	q := NewRemoteUnordered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewPriorityDriver()
	assert.NoError(d.Open(ctx))
	assert.NoError(q.SetDriver(d))

	runUnorderedSmokeTest(ctx, q, 1, assert)
	d.Close()
}

func TestSmokePriorityDriverWithRemoteQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 6, 7, 16, 32, 64} {
		grip.Infoln("testing priority queue for:", poolSize)
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q := NewRemoteUnordered(poolSize)
		d := NewPriorityDriver()
		assert.NoError(q.SetDriver(d))
		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
		d.Close()
	}
}

func TestSmokeMultipleMgoDriverRemoteUnorderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	dOne := NewMgoDriver(name+"-one", opts).(*mgoDriver)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	dTwo := NewMgoDriver(name+"-two", opts).(*mgoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// release runner/driver resources.
	cancel()

	// do cleanup.
	grip.Error(cleanupMgo(opts.DB, name, dOne.session.Clone()))
}

func TestSmokeMultipleMongoDriverRemoteUnorderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	dOne := NewMongoDriver(name+"-one", opts).(*mongoDriver)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	dTwo := NewMongoDriver(name+"-two", opts).(*mongoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// do cleanup.
	grip.Error(cleanupMongo(ctx, opts.DB, name, dOne.client))

	// release runner/driver resources.
	cancel()
}

func TestSmokeMultipleMgoBackedRemoteUnorderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	dOne := NewMgoDriver(name+"-one", opts).(*mgoDriver)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	dTwo := NewMgoDriver(name+"-two", opts).(*mgoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// release runner/driver resources.
	cancel()

	// do cleanup.
	grip.Error(cleanupMgo(opts.DB, name, dOne.session.Clone()))
}

func TestSmokeMultipleMongoBackedRemoteUnorderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	dOne := NewMongoDriver(name+"-one", opts).(*mongoDriver)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	dTwo := NewMongoDriver(name+"-two", opts).(*mongoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// do cleanup.
	grip.Error(cleanupMongo(ctx, opts.DB, name, dOne.client))

	// release runner/driver resources.
	cancel()
}

func TestSmokeMultipleLocalBackedRemoteOrderedQueuesWithOneDriver(t *testing.T) {
	if os.Getenv("EVR_TASK_ID") != "" {
		t.Skip("skipping weird test evergreen, only where it deadlocks.",
			"this failure is difficult to debug",
			"the is useful for validating the driver/remote queue interface, but isn't useful")
	}

	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	qOne := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	qTwo := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	d := NewInternalDriver()
	assert.NoError(qOne.SetDriver(d))
	assert.NoError(qTwo.SetDriver(d))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, true, assert)
	defer cancel()
	d.Close()
}

func TestSmokeMultipleMgoDriverRemoteOrderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	dOne := NewMgoDriver(name+"-one", opts).(*mgoDriver)
	qTwo := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	dTwo := NewMgoDriver(name+"-two", opts).(*mgoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// release runner/driver resources.
	cancel()

	// do cleanup.
	grip.Error(cleanupMgo(opts.DB, name, dOne.session.Clone()))
}

func TestSmokeMultipleMongoDriverRemoteOrderedQueuesWithTheSameName(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	// create queues with two runners, mongodb backed drivers, and
	// configure injectors
	qOne := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	dOne := NewMongoDriver(name+"-one", opts).(*mongoDriver)
	qTwo := NewSimpleRemoteOrdered(runtime.NumCPU() / 2)
	dTwo := NewMongoDriver(name+"-two", opts).(*mongoDriver)
	assert.NoError(dOne.Open(ctx))
	assert.NoError(dTwo.Open(ctx))
	assert.NoError(qOne.SetDriver(dOne))
	assert.NoError(qTwo.SetDriver(dTwo))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, false, assert)

	// do cleanup.
	grip.Error(cleanupMongo(ctx, opts.DB, name, dOne.client))
}

func TestSmokeMultipleLocalBackedRemoteUnorderedQueuesWithOneDriver(t *testing.T) {
	if os.Getenv("EVR_TASK_ID") != "" {
		t.Skip("skipping weird test evergreen, only where it deadlocks.",
			"this failure is difficult to debug",
			"the is useful for validating the driver/remote queue interface, but isn't useful")
	}

	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	d := NewInternalDriver()
	assert.NoError(qOne.SetDriver(d))
	assert.NoError(qTwo.SetDriver(d))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, true, assert)
}

func TestSmokeMultipleQueuesWithPriorityDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	qOne := NewRemoteUnordered(runtime.NumCPU() / 2)
	qTwo := NewRemoteUnordered(runtime.NumCPU() / 2)
	d := NewPriorityDriver()
	assert.NoError(qOne.SetDriver(d))
	assert.NoError(qTwo.SetDriver(d))

	runMultiQueueSingleBackEndSmokeTest(ctx, qOne, qTwo, true, assert)
}

func TestSmokeLimitedSizeQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewLocalLimitedSize(1, 1024)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))

	assert.NoError(q.SetRunner(runner))

	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestSmokeLimitedSizeQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 6, 7, 16, 32, 64} {
		grip.Infoln("testing priority queue for:", poolSize)
		ctx, cancel := context.WithCancel(baseCtx)

		q := NewLocalLimitedSize(poolSize, 7*poolSize+1)
		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeShuffledQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewShuffledLocal(1, defaultLocalQueueCapcity)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))

	assert.NoError(q.SetRunner(runner))

	runUnorderedSmokeTest(ctx, q, 1, assert)
}

func TestFixedSizeShuffledQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewShuffledLocal(4, 50)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))

	assert.NoError(q.SetRunner(runner))

	num := populateUnorderedSmokeTest(ctx, q, 100, assert)
	stat := q.Stats()
	assert.NotEqual(num, stat.Total)
	assert.Equal(50, stat.Completed)
	assert.Equal(50, stat.Total)
}

func TestSmokeShuffledQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 6, 7, 16, 32, 64} {
		grip.Infoln("testing shuffled queue for:", poolSize)
		ctx, cancel := context.WithCancel(baseCtx)

		q := NewShuffledLocal(poolSize, defaultLocalQueueCapcity)

		runUnorderedSmokeTest(ctx, q, poolSize, assert)

		cancel()
	}
}

func TestSmokeSimpleRemoteOrderedWorkerPoolsWithMgoDriver(t *testing.T) {
	t.Skip("This test is flakey, often deadlocks, and tests a weird combination of workloads.")

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	baseCtx := context.Background()

	session, err := mgo.DialWithTimeout(opts.URI, 5*time.Second)
	assert.NoError(err)
	defer session.Close()

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		start := time.Now()
		grip.Infof("running mongodb simple ordered queue smoke test with %d jobs", poolSize)
		q := NewSimpleRemoteOrdered(poolSize)
		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

		ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
		d, err := OpenNewMgoDriver(ctx, name, opts, session.Copy())
		assert.NoError(err)
		assert.NoError(q.SetDriver(d))

		runUnorderedSmokeTest(ctx, q, poolSize, assert)
		cancel()
		d.Close()

		grip.Infof("test with %d jobs, duration = %s", poolSize, time.Since(start))
		err = cleanupMgo(opts.DB, name, session.Clone())
		grip.AlertWhenf(err != nil, "encountered error cleaning up %s: %+v", name, err)
	}
}

func TestSmokeSimpleRemoteOrderedWithSingleThreadedAndMgoDriver(t *testing.T) {
	t.Skip("This test is flakey, often deadlocks, and tests a weird combination of workloads.")

	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewSimpleRemoteOrdered(1)
	d := NewMgoDriver(name, opts).(*mgoDriver)
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	// intentionally running the ordered
	runUnorderedSmokeTest(ctx, q, 1, assert)
	cancel()
	d.Close()
	grip.Error(cleanupMgo(opts.DB, name, d.session.Clone()))
}

func TestSmokeSimpleRemoteOrderedWorkerPoolsWithInternalDriver(t *testing.T) {
	if os.Getenv("EVR_TASK_ID") != "" {
		t.Skip("skipping weird test evergreen, only where it deadlocks.",
			"this failure is difficult to debug",
			"the is useful for validating the driver/remote queue interface, but isn't useful")
	}

	assert := assert.New(t) // nolint
	baseCtx := context.Background()

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithTimeout(baseCtx, time.Minute)

		q := NewSimpleRemoteOrdered(poolSize)
		d := NewInternalDriver()
		assert.NoError(q.SetDriver(d))
		runOrderedSmokeTest(ctx, q, poolSize, false, assert)

		cancel()
		d.Close()
	}
}

func TestSmokeSimpleRemoteOrderedWithSingleRunnerAndMgoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewSimpleRemoteOrdered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewMgoDriver(name, opts).(*mgoDriver)
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runOrderedSmokeTest(ctx, q, 1, false, assert)
	cancel()
	d.Close()
	grip.Error(cleanupMgo(opts.DB, name, d.session.Clone()))
}

func TestSmokeSimpleRemoteOrderedWithSingleRunnerAndMongoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)

	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	q := NewSimpleRemoteOrdered(1)

	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewMongoDriver(name, opts).(*mongoDriver)
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runOrderedSmokeTest(ctx, q, 1, false, assert)
	d.Close()
	grip.Error(cleanupMongo(ctx, opts.DB, name, d.client))
	cancel()
}

func TestSmokeSimpleRemoteOrderedWithSingleThreadedAndInternalDriver(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	q := NewSimpleRemoteOrdered(1)
	d := NewInternalDriver()
	assert.NoError(d.Open(ctx))

	assert.NoError(q.SetDriver(d))

	runOrderedSmokeTest(ctx, q, 1, false, assert)
	cancel()
	d.Close()
}

func TestSmokeSimpleRemoteOrderedWithSingleRunnerAndInternalDriver(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	q := NewSimpleRemoteOrdered(1)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	d := NewInternalDriver()
	assert.NoError(d.Open(ctx))
	assert.NoError(q.SetDriver(d))

	runOrderedSmokeTest(ctx, q, 1, false, assert)
	cancel()
	d.Close()
}

func TestSmokeLocalOrderedQueueWithWorkerPools(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewLocalOrdered(poolSize)
		runOrderedSmokeTest(ctx, q, poolSize, false, assert)
		cancel()
	}
}

func TestSmokeLocalOrderedQueueWithSingleWorker(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := NewLocalOrdered(1)
	runner := pool.NewSingle()
	assert.NoError(runner.SetQueue(q))
	assert.NoError(q.SetRunner(runner))

	runOrderedSmokeTest(ctx, q, 1, false, assert)
}

func TestSmokeAdaptiveOrderingWithOrderedWorkAndVariablePools(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewAdaptiveOrderedLocalQueue(poolSize, defaultLocalQueueCapcity)

		runOrderedSmokeTest(ctx, q, poolSize, true, assert)
		cancel()
	}
}

func TestSmokeAdaptiveOrderingWithUnorderedWorkAndVariablePools(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewAdaptiveOrderedLocalQueue(poolSize, defaultLocalQueueCapcity)

		runUnorderedSmokeTest(ctx, q, poolSize, assert)
		cancel()
	}
}

func TestSmokeAdaptiveOrderingWithOrderedWorkAndSinglePools(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	q := NewAdaptiveOrderedLocalQueue(1, defaultLocalQueueCapcity)
	assert.NoError(q.SetRunner(pool.NewSingle()))

	runOrderedSmokeTest(ctx, q, 1, true, assert)
	cancel()
}

func TestSmokeAdaptiveOrderingWithUnorderedWorkAndSinglePools(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx, cancel := context.WithCancel(context.Background())
	q := NewAdaptiveOrderedLocalQueue(1, defaultLocalQueueCapcity)
	assert.NoError(q.SetRunner(pool.NewSingle()))
	runUnorderedSmokeTest(ctx, q, 1, assert)
	cancel()
}

func TestSmokeRemoteOrderedWithWorkerPoolsAndMgoDriver(t *testing.T) {
	t.Skip("this test test's an odd combination of variables, and gets stuck")

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	session, err := mgo.DialWithTimeout(opts.URI, 5*time.Second)
	assert.NoError(err)
	defer session.Close()

	for _, poolSize := range []int{2, 4, 8, 16, 32} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		q := NewSimpleRemoteOrdered(poolSize)

		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)
		driver, err := OpenNewMgoDriver(ctx, name, opts, session.Clone())
		assert.NoError(err)
		assert.NoError(q.SetDriver(driver))
		defer driver.Close()

		runOrderedSmokeTest(ctx, q, poolSize, false, assert)
		cancel()
		grip.Error(cleanupMgo(opts.DB, name, session.Clone()))
	}
}

func TestSmokeRemoteOrderedWithWorkerPoolsAndMongoDriver(t *testing.T) {
	t.Skip("this test test's an odd combination of variables, and gets stuck")

	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.URI).SetConnectTimeout(5*time.Second))
	require.NoError(t, err)

	for _, poolSize := range []int{2, 4, 8, 16, 32} {
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		q := NewSimpleRemoteOrdered(poolSize)

		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)
		driver, err := OpenNewMongoDriver(ctx, name, opts, client)
		assert.NoError(err)
		assert.NoError(q.SetDriver(driver))
		defer driver.Close()

		runOrderedSmokeTest(ctx, q, poolSize, false, assert)
		grip.Error(cleanupMongo(ctx, opts.DB, name, client))
		cancel()
	}
}

func TestSmokeWaitUntilAdaptiveOrderQueuePools(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{1, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewAdaptiveOrderedLocalQueue(poolSize, defaultLocalQueueCapcity)
		runWaitUntilSmokeTest(ctx, q, poolSize, assert)
		cancel()
	}
}

func TestSmokeWaitUntilMgoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	opts.CheckWaitUntil = true

	for _, poolSize := range []int{4, 8} {
		ctx, cancel := context.WithCancel(context.Background())

		q := NewSimpleRemoteOrdered(poolSize)

		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)
		driver := NewMgoDriver(name, opts).(*mgoDriver)
		assert.NoError(driver.Open(ctx))
		assert.NoError(q.SetDriver(driver))

		runWaitUntilSmokeTest(ctx, q, poolSize, assert)
		cancel()
		driver.Close()
		grip.Error(cleanupMgo(opts.DB, name, driver.session.Clone()))
	}
}

func TestSmokeWaitUntilMongoDriver(t *testing.T) {
	assert := assert.New(t) // nolint
	opts := DefaultMongoDBOptions()
	opts.DB = "amboy_test"
	opts.CheckWaitUntil = true

	for _, poolSize := range []int{4, 8} {
		ctx, cancel := context.WithCancel(context.Background())

		q := NewSimpleRemoteOrdered(poolSize)

		name := strings.Replace(uuid.NewV4().String(), "-", ".", -1)
		driver := NewMongoDriver(name, opts).(*mongoDriver)
		assert.NoError(driver.Open(ctx))
		assert.NoError(q.SetDriver(driver))

		runWaitUntilSmokeTest(ctx, q, poolSize, assert)
		driver.Close()
		grip.Error(cleanupMongo(ctx, opts.DB, name, driver.client))
		cancel()
	}
}

func TestSmokeRemoteOrderedWithWorkerPoolsAndLocalDriver(t *testing.T) {
	assert := assert.New(t) // nolint

	for _, poolSize := range []int{2, 4, 8, 16, 32, 64} {
		ctx, cancel := context.WithCancel(context.Background())
		q := NewSimpleRemoteOrdered(poolSize)

		driver := NewInternalDriver()
		assert.NoError(q.SetDriver(driver))

		runOrderedSmokeTest(ctx, q, poolSize, true, assert)
		cancel()
	}
}

func cleanupMgo(dbname, name string, session *mgo.Session) error {
	start := time.Now()
	defer session.Close()

	if err := session.DB(dbname).C(name + ".jobs").DropCollection(); err != nil {
		return errors.WithStack(err)
	}

	grip.Infof("clean up operation for %s took %s", name, time.Since(start))
	return nil
}

func cleanupMongo(ctx context.Context, dbname, name string, client *mongo.Client) error {
	start := time.Now()

	if err := client.Database(dbname).Collection(name + ".jobs").Drop(ctx); err != nil {
		return errors.WithStack(err)
	}

	grip.Infof("clean up operation for %s took %s", name, time.Since(start))
	return nil
}
