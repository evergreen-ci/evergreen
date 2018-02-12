package queue

import (
	"context"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LocalQueueSuite struct {
	size    int
	queue   *unorderedLocal
	require *require.Assertions
	suite.Suite
}

func TestLocalQueueSuiteOneWorker(t *testing.T) {
	s := &LocalQueueSuite{}
	s.size = 1
	suite.Run(t, s)
}

func TestLocalQueueSuiteThreeWorkers(t *testing.T) {
	s := &LocalQueueSuite{}
	s.size = 3
	suite.Run(t, s)
}

func (s *LocalQueueSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *LocalQueueSuite) SetupTest() {
	s.queue = NewLocalUnordered(s.size).(*unorderedLocal)
}

func (s *LocalQueueSuite) TestDefaultStateOfQueueObjectIsExpected() {
	s.False(s.queue.started)

	s.Len(s.queue.tasks.m, 0)

	s.NotNil(s.queue.runner)
}

func (s *LocalQueueSuite) TestPutReturnsErrorForDuplicateNameTasks() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("true", "")

	s.NoError(s.queue.Start(ctx))

	s.NoError(s.queue.Put(j))
	s.Error(s.queue.Put(j))
}

func (s *LocalQueueSuite) TestPuttingAJobIntoAQueueImpactsStats() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stats := s.queue.Stats()
	s.Equal(0, stats.Total)
	s.Equal(0, stats.Pending)
	s.Equal(0, stats.Running)
	s.Equal(0, stats.Completed)

	s.NoError(s.queue.Start(ctx))

	j := job.NewShellJob("true", "")
	s.NoError(s.queue.Put(j))

	jReturn, ok := s.queue.Get(j.ID())
	s.True(ok)
	s.Exactly(jReturn, j)

	stats = s.queue.Stats()
	s.Equal(1, stats.Total)
	s.Equal(1, stats.Pending)
	s.Equal(1, stats.Running)
	s.Equal(0, stats.Completed)
}

func (s *LocalQueueSuite) TestResultsChannelProducesPointersToConsistentJobObjects() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("true", "")
	s.False(j.Status().Completed)

	s.NoError(s.queue.Start(ctx))
	s.NoError(s.queue.Put(j))

	amboy.Wait(s.queue)

	result, ok := <-s.queue.Results(ctx)
	s.True(ok)
	s.Equal(j.ID(), result.ID())
	s.True(result.Status().Completed)
}

func (s *LocalQueueSuite) TestJobsChannelProducesJobObjects() {
	names := map[string]bool{"ru": true, "es": true, "zh": true, "fr": true, "it": true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// this test hinges on the queue system, and this
	// implementation in particular, being FIFO.
	s.NoError(s.queue.Start(ctx))

	for name := range names {
		j := job.NewShellJob("echo "+name, "")
		s.NoError(s.queue.Put(j))
	}

	amboy.Wait(s.queue)

	for j := range s.queue.Results(ctx) {
		shellJob, ok := j.(*job.ShellJob)
		s.True(ok)
		s.True(names[shellJob.Output])
	}
}

func (s *LocalQueueSuite) TestInternalRunnerCanBeChangedBeforeStartingTheQueue() {
	originalRunner := s.queue.Runner()
	newRunner := pool.NewLocalWorkers(2, s.queue)
	s.NotEqual(originalRunner, newRunner)

	s.NoError(s.queue.SetRunner(newRunner))
	s.Exactly(newRunner, s.queue.Runner())
}

func (s *LocalQueueSuite) TestInternalRunnerCannotBeChangedAfterStartingAQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := s.queue.Runner()
	s.False(s.queue.Started())
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())

	newRunner := pool.NewLocalWorkers(2, s.queue)
	s.Error(s.queue.SetRunner(newRunner))
	s.NotEqual(runner, newRunner)
}

func (s *LocalQueueSuite) TestQueueCanOnlyBeStartedOnce() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.False(s.queue.Started())
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())

	amboy.Wait(s.queue)
	s.True(s.queue.Started())

	// you can call start more than once until the queue has
	// completed/closed
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())
}

func (s *LocalQueueSuite) TestPutReturnsErrorIfQueueIsNotStarted() {
	s.False(s.queue.started)
	j := job.NewShellJob("true", "")
	s.Error(s.queue.Put(j))
}
