package pool

import (
	"context"
	"testing"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
)

func TestSimpleRateLimitingConstructor(t *testing.T) {
	var (
		runner amboy.Runner
		err    error
	)

	assert := assert.New(t)
	queue := &QueueTester{
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}

	runner, err = NewSimpleRateLimitedWorkers(1, time.Nanosecond, nil)
	assert.Nil(runner)
	assert.Error(err)
	assert.Contains(err.Error(), "less than a millisecond")
	assert.Contains(err.Error(), "nil queue")

	runner, err = NewSimpleRateLimitedWorkers(0, time.Millisecond, nil)
	assert.Nil(runner)
	assert.Error(err)
	assert.Contains(err.Error(), "pool size less than 1")
	assert.Contains(err.Error(), "nil queue")

	runner, err = NewSimpleRateLimitedWorkers(10, 10*time.Millisecond, queue)
	assert.NoError(err)
	assert.NotNil(runner)
}

func TestAverageRateLimitingConstructor(t *testing.T) {
	assert := assert.New(t) // nolint

	var (
		runner amboy.Runner
		err    error
	)

	queue := &QueueTester{
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}

	runner, err = NewMovingAverageRateLimitedWorkers(1, 0, time.Nanosecond, nil)
	assert.Nil(runner)
	assert.Error(err)
	assert.Contains(err.Error(), "less than a millisecond")
	assert.Contains(err.Error(), "target number of tasks less than 1")
	assert.Contains(err.Error(), "nil queue")

	runner, err = NewMovingAverageRateLimitedWorkers(0, 1, time.Millisecond, nil)
	assert.Nil(runner)
	assert.Error(err)
	assert.Contains(err.Error(), "pool size less than 1")
	assert.Contains(err.Error(), "nil queue")

	runner, err = NewMovingAverageRateLimitedWorkers(4, 10, 10*time.Millisecond, queue)
	assert.NoError(err)
	assert.NotNil(runner)
}

func TestAvergeTimeCalculator(t *testing.T) {
	assert := assert.New(t) // nolint

	p := ewmaRateLimiting{
		ewma:   ewma.NewMovingAverage(),
		period: time.Minute,
		size:   2,
		target: 10,
	}
	// average is uninitialized by default
	assert.Equal(p.ewma.Value(), float64(0))

	// some initial setup, sanity check an actual value
	result := p.getNextTime(time.Millisecond)
	assert.InDelta(5*time.Second, result, float64(time.Second), "actual:%s", result)
	result = p.getNextTime(time.Minute)

	// priming the average and watching the return value of the
	// function increase:
	//
	// getNexttime returns how much time the worker loop should
	// sleep between jobs, as a result of the average execution
	// time of a task going down from the ~minute used above, the
	// amount of time needed to spend sleeping is going *up* which
	// means the values are going up in this function.
	var last time.Duration
	for i := 0; i < 100; i++ {
		result = p.getNextTime(time.Second)

		assert.True(last <= result, "%d:%s<=%s", i, last, result)
		last = result
	}

	assert.True(p.getNextTime(time.Second) > time.Second)

	// also run tests of the wrapper runJobs function which executes tasks and calls getNextTime
	p.queue = &QueueTester{
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := job.NewShellJob("hostname", "")
	assert.False(j.Status().Completed)
	assert.True(p.runJob(ctx, j) > time.Nanosecond)
	assert.True(j.Status().Completed)

	// mess with the target number of tasks to make sure that we
	// get 0 wait time if there's no time needed between tasks
	p.target = 100000
	assert.Equal(p.getNextTime(time.Millisecond), time.Duration(0))
	p.target = 10

	// duration is larger than period, returns zero
	assert.Equal(p.getNextTime(time.Hour), time.Duration(0))

}

func TestSimpleRateLimitingWorkerHandlesPanicingJobs(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p := &simpleRateLimited{}
	p.queue = &QueueTester{
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}
	assert.NotPanics(func() { p.worker(ctx, jobsChanWithPanicingJobs(ctx, 10)) })
}

func TestEWMARateLimitingWorkerHandlesPanicingJobs(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p := &ewmaRateLimiting{}
	p.queue = &QueueTester{
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}
	assert.NotPanics(func() { p.worker(ctx, jobsChanWithPanicingJobs(ctx, 10)) })
}

func TestMultipleWorkers(t *testing.T) {
	assert := assert.New(t) // nolint
	for workers := time.Duration(1); workers <= 10; workers++ {
		ema := ewmaRateLimiting{
			period: time.Minute,
			target: 60,
			size:   int(workers),
			queue:  nil,
			ewma:   ewma.NewMovingAverage(),
		}
		for i := 0; i < 100; i++ {
			next := ema.getNextTime(time.Millisecond)
			if !assert.True(next*workers > 750*time.Millisecond) || !assert.True(next < workers*time.Second) {
				grip.Errorf("workers=%d, iter=%d, next=%s", workers, i, next)
			}

			// sam's test
			assert.InDelta(time.Duration(workers)*time.Second, float64(next), float64(workers*10*time.Millisecond),
				"next=%s, workers=%d, iter=%d", next, workers, i)

			// brian's test:
			assert.InDelta(time.Duration(workers)*time.Second, next, float64(100*time.Millisecond),
				"next=%s, workers=%d, iter=%d", next, workers, i)
		}
	}
}
