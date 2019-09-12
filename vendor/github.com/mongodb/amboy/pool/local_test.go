package pool

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LocalWorkersSuite struct {
	size  int
	pool  *localWorkers
	queue *QueueTester
	suite.Suite
}

func TestLocalWorkersSuiteSizeOne(t *testing.T) {
	s := new(LocalWorkersSuite)
	s.size = 1

	suite.Run(t, s)
}

func TestLocalWorkersSuiteSizeThree(t *testing.T) {
	s := new(LocalWorkersSuite)
	s.size = 3

	suite.Run(t, s)
}

func TestLocalWorkersSuiteSizeOneHundred(t *testing.T) {
	s := new(LocalWorkersSuite)
	s.size = 100

	suite.Run(t, s)
}

func (s *LocalWorkersSuite) SetupTest() {
	s.pool = NewLocalWorkers(s.size, nil).(*localWorkers)
	s.queue = NewQueueTester(s.pool)
}

func (s *LocalWorkersSuite) TestPanicJobsDoNotPanicHarness() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg := &sync.WaitGroup{}

	s.queue.toProcess = jobsChanWithPanicingJobs(ctx, s.size)
	s.NotPanics(func() { worker(ctx, "test-local", s.queue, wg) })
}

func (s *LocalWorkersSuite) TestConstructedInstanceImplementsInterface() {
	s.Implements((*amboy.Runner)(nil), s.pool)
}

func (s *LocalWorkersSuite) TestPoolErrorsOnSuccessiveStarts() {
	s.False(s.pool.Started())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.pool.Start(ctx))
	s.True(s.pool.Started())

	for i := 0; i < 20; i++ {
		s.NoError(s.pool.Start(ctx))
		s.True(s.pool.Started())
	}
}

func (s *LocalWorkersSuite) TestPoolStartsAndProcessesJobs() {
	const num int = 100
	var jobs []amboy.Job

	for i := 0; i < num; i++ {
		cmd := fmt.Sprintf("echo 'task=%d'", i)
		jobs = append(jobs, job.NewShellJob(cmd, ""))
	}

	s.False(s.pool.Started())
	s.False(s.queue.Started())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.queue.Start(ctx))

	for _, job := range jobs {
		s.NoError(s.queue.Put(ctx, job))
	}

	s.True(s.pool.Started())
	s.True(s.queue.Started())

	amboy.WaitInterval(ctx, s.queue, 100*time.Millisecond)

	counter := 0
	for j := range s.queue.Results(ctx) {
		s.True(j.Status().Completed)
		counter++
	}
	s.Equal(counter, len(jobs))

	for _, job := range jobs {
		s.True(job.Status().Completed)
	}
}

func (s *LocalWorkersSuite) TestQueueIsMutableBeforeStartingPool() {
	s.NotNil(s.pool.queue)
	s.False(s.pool.Started())

	newQueue := NewQueueTester(s.pool)
	s.NoError(s.pool.SetQueue(newQueue))

	s.Equal(newQueue, s.pool.queue)
	s.NotEqual(s.queue, s.pool.queue)
}

func (s *LocalWorkersSuite) TestQueueIsNotMutableAfterStartingPool() {
	s.NotNil(s.pool.queue)
	s.False(s.pool.Started())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.pool.Start(ctx))
	s.True(s.pool.Started())

	newQueue := NewQueueTester(s.pool)
	s.Error(s.pool.SetQueue(newQueue))

	s.Equal(s.queue, s.pool.queue)
	s.NotEqual(newQueue, s.pool.queue)
}

// This test makes sense to do without the fixtures in the suite

func TestLocalWorkerPoolConstructorDoesNotAllowSizeValuesLessThanOne(t *testing.T) {
	assert := assert.New(t)
	var pool *localWorkers
	var runner amboy.Runner

	for _, size := range []int{-10, -1, 0} {
		runner = NewLocalWorkers(size, nil)
		pool = runner.(*localWorkers)

		assert.Equal(1, pool.size)
	}
}

func TestPanicJobPanics(t *testing.T) {
	assert := assert.New(t) // nolint
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for job := range jobsChanWithPanicingJobs(ctx, 8) {
		assert.Panics(func() { job.Run(ctx) })
	}

}
