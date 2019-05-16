package queue

import (
	"context"
	"fmt"
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
	suite.Run(t, new(SQSFifoQueueSuite))
}

func (s *SQSFifoQueueSuite) SetupTest() {
	var err error
	s.queue, err = NewSQSFifoQueue(randomString(4), 4)
	s.NoError(err)
	r := pool.NewSingle()
	s.NoError(r.SetQueue(s.queue))
	s.NoError(s.queue.SetRunner(r))
	s.Equal(r, s.queue.Runner())
	s.NoError(s.queue.Start(context.Background()))

	stats := s.queue.Stats()
	s.Equal(0, stats.Total)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Completed)

	j := job.NewShellJob("echo true", "")
	s.jobID = j.ID()
	s.NoError(s.queue.Put(j))
}

func (s *SQSFifoQueueSuite) TestPutMethodErrorsForDuplicateJobs() {
	job, ok := s.queue.Get(s.jobID)
	s.True(ok)
	s.Error(s.queue.Put(job))
}

func (s *SQSFifoQueueSuite) TestGetMethodReturnsRequestedJob() {
	job, ok := s.queue.Get(s.jobID)
	s.True(ok)
	s.NotNil(job)
	s.Equal(s.jobID, job.ID())
}

func (s *SQSFifoQueueSuite) TestCannotSetRunnerWhenQueueStarted() {
	s.True(s.queue.Started())
	s.Error(s.queue.SetRunner(pool.NewSingle()))
}

func (s *SQSFifoQueueSuite) TestCompleteMethodChangesStatsAndResults() {
	j := job.NewShellJob("echo true", "")
	s.NoError(s.queue.Put(j))
	s.queue.Complete(context.Background(), j)

	counter := 0
	results := s.queue.Results(context.Background())
	for job := range results {
		s.Require().NotNil(job)
		s.Equal(j.ID(), job.ID())
		counter++
	}
	stats := s.queue.Stats()
	s.Equal(1, stats.Completed)
	s.Equal(1, counter)
}

func TestSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
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
				assert.NoError(q.Put(j))
			}
		}(i)
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	amboy.WaitCtxInterval(ctx, q, 20*time.Second)
	stats := q.Stats()
	assert.True(stats.Total <= inside*outside)
	assert.Equal(stats.Total, stats.Pending+stats.Completed)
}

func TestMultipleSQSFifoQueueRunsJobsOnlyOnce(t *testing.T) {
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
				assert.NoError(q2.Put(j))
			}
		}(i)
	}
	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	amboy.WaitCtxInterval(ctx, q, 10*time.Second)
	amboy.WaitCtxInterval(ctx, q2, 10*time.Second)

	stats1 := q.Stats()
	stats2 := q2.Stats()
	assert.Equal(stats1.Pending, stats2.Pending)

	sum := stats1.Pending + stats2.Pending + stats1.Completed + stats2.Completed
	assert.Equal(sum, stats1.Total+stats2.Total)
}
