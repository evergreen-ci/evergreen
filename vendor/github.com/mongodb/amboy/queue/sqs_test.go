package queue

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SQSFifoQueueSuite struct {
	queue amboy.Queue
	suite.Suite
	jobID string
}

func TestSQSFifoQueueSuite(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("skipping SQS tests on non-core platforms")
	}
	suite.Run(t, new(SQSFifoQueueSuite))
}

func (s *SQSFifoQueueSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	s.queue, err = NewSQSFifoQueue(randomString(4), 4)
	s.NoError(err)
	r := pool.NewSingle()
	s.NoError(r.SetQueue(s.queue))
	s.NoError(s.queue.SetRunner(r))
	s.Equal(r, s.queue.Runner())
	s.NoError(s.queue.Start(ctx))

	stats := s.queue.Stats(ctx)
	s.Equal(0, stats.Total)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Completed)

	j := job.NewShellJob("echo true", "")
	s.jobID = j.ID()
	s.NoError(s.queue.Put(ctx, j))
}

func (s *SQSFifoQueueSuite) TestPutMethodErrorsForDuplicateJobs() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job, ok := s.queue.Get(ctx, s.jobID)
	s.True(ok)
	s.Error(s.queue.Put(ctx, job))
}

func (s *SQSFifoQueueSuite) TestGetMethodReturnsRequestedJob() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job, ok := s.queue.Get(ctx, s.jobID)
	s.True(ok)
	s.NotNil(job)
	s.Equal(s.jobID, job.ID())
}

func (s *SQSFifoQueueSuite) TestCannotSetRunnerWhenQueueStarted() {
	s.True(s.queue.Started())
	s.Error(s.queue.SetRunner(pool.NewSingle()))
}

func (s *SQSFifoQueueSuite) TestCompleteMethodChangesStatsAndResults() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("echo true", "")
	s.NoError(s.queue.Put(ctx, j))
	s.queue.Complete(context.Background(), j)

	counter := 0
	results := s.queue.Results(context.Background())
	for job := range results {
		s.Require().NotNil(job)
		s.Equal(j.ID(), job.ID())
		counter++
	}
	stats := s.queue.Stats(ctx)
	s.Equal(1, stats.Completed)
	s.Equal(1, counter)
}

func TestSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("skipping SQS tests on non-core platforms")
	}

	assert := assert.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	q, err := NewSQSFifoQueue(randomString(8), 4)
	assert.NoError(err)
	assert.NoError(q.Start(ctx))
	wg := &sync.WaitGroup{}

	const (
		inside  = 250
		outside = 2
	)

	wg.Add(outside)
	for i := 0; i < outside; i++ {
		go func(i int) {
			defer wg.Done()
			for ii := 0; ii < inside; ii++ {
				j := newMockJob()
				j.SetID(fmt.Sprintf("%d-%d-%d", i, ii, job.GetNumber()))
				assert.NoError(q.Put(ctx, j))
			}
		}(i)
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	amboy.WaitInterval(ctx, q, 20*time.Second)
	stats := q.Stats(ctx)
	assert.True(stats.Total <= inside*outside)
	assert.Equal(stats.Total, stats.Pending+stats.Completed)
}

func TestMultipleSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("skipping SQS tests on non-core platforms")
	}

	// case two
	mockJobCounters.Reset()

	assert := assert.New(t) // nolint
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	defer cancel()
	name := randomString(8)
	q, err := NewSQSFifoQueue(name, 4)
	assert.NoError(err)
	assert.NoError(q.Start(ctx))

	q2, err := NewSQSFifoQueue(name, 4)
	assert.NoError(err)
	assert.NoError(q2.Start(ctx))

	const (
		inside  = 250
		outside = 3
	)

	wg := &sync.WaitGroup{}
	wg.Add(outside)
	for i := 0; i < outside; i++ {
		go func(i int) {
			defer wg.Done()
			for ii := 0; ii < inside; ii++ {
				j := newMockJob()
				j.SetID(fmt.Sprintf("%d-%d-%d", i, ii, job.GetNumber()))
				assert.NoError(q2.Put(ctx, j))
			}
		}(i)
	}
	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	amboy.WaitInterval(ctx, q, 10*time.Second)
	amboy.WaitInterval(ctx, q2, 10*time.Second)

	stats1 := q.Stats(ctx)
	stats2 := q2.Stats(ctx)
	assert.Equal(stats1.Pending, stats2.Pending)

	sum := stats1.Pending + stats2.Pending + stats1.Completed + stats2.Completed
	assert.Equal(sum, stats1.Total+stats2.Total)
}
