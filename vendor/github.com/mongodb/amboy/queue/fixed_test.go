package queue

import (
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

// LocalLimitedSizeQueue suite tests the queue implementation that
// uses the CappedResultStorage. These tests exercise the aspects of
// this queue implementation that are not covered by the tests of the
// storage object *or* exercised by the general queue functionality
// tests, which test all implementations.
type LimitedSizeQueueSuite struct {
	queue       *LocalLimitedSize
	numWorkers  int
	numCapacity int
	require     *require.Assertions
	suite.Suite
}

func TestLimitedSizeQueueSuite(t *testing.T) {
	suite.Run(t, new(LimitedSizeQueueSuite))
}

func (s *LimitedSizeQueueSuite) SetupSuite() {
	s.numWorkers = 2
	s.numCapacity = 100
	s.require = s.Require()
}

func (s *LimitedSizeQueueSuite) SetupTest() {
	s.queue = NewLocalLimitedSize(s.numWorkers, s.numCapacity)
}

func (s *LimitedSizeQueueSuite) TestBufferForPendingWorkEqualToCapacityForResults() {
	s.False(s.queue.Started())
	s.queue.Runner().Close()
	s.Nil(s.queue.channel)
	s.Error(s.queue.Put(job.NewShellJob("sleep 10", "")))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.queue.Start(ctx))
	for i := 0; i < s.numCapacity+s.numWorkers+1; i++ {
		err := s.queue.Put(job.NewShellJob("sleep 10", ""))
		if len(s.queue.channel) != s.numCapacity {
			s.NoError(err)
		}
	}

	s.Len(s.queue.channel, s.numCapacity)
	s.Error(s.queue.Put(job.NewShellJob("sleep 10", "")))
}

func (s *LimitedSizeQueueSuite) TestCallingStartMultipleTimesDoesNotImpactState() {
	s.False(s.queue.Started())
	s.Nil(s.queue.channel)
	ctx := context.Background()
	s.NoError(s.queue.Start(ctx))

	for i := 0; i < 100; i++ {
		s.NoError(s.queue.Start(ctx))
	}
}

func (s *LimitedSizeQueueSuite) TestCannotSetRunnerAfterQueueIsOpened() {
	secondRunner := pool.NewSingle()
	runner := s.queue.runner

	s.False(s.queue.Started())
	for i := 0; i < 25; i++ {
		s.NoError(s.queue.SetRunner(secondRunner))
		s.NoError(s.queue.SetRunner(runner))
	}
	s.False(s.queue.Started())

	ctx := context.Background()
	s.NoError(s.queue.Start(ctx))

	s.True(s.queue.Started())

	for i := 0; i < 30; i++ {
		s.Error(s.queue.SetRunner(secondRunner))
		s.Error(s.queue.SetRunner(runner))
	}
}

func (s *LimitedSizeQueueSuite) TestGetMethodOnlyReturnsCompletedJobs() {
	s.False(s.queue.Started())

	ctx := context.Background()
	s.NoError(s.queue.Start(ctx))

	s.True(s.queue.Started())

	jobs := make(map[string]amboy.Job)
	for i := 0; i < s.numCapacity; i++ {
		j := job.NewShellJob("true", "")
		s.NoError(s.queue.Put(j))
		jobs[j.ID()] = j
	}

	amboy.Wait(s.queue)
	s.queue.Runner().Close()

	for name, j := range jobs {
		rj, ok := s.queue.Get(name)
		s.True(ok)
		s.NotNil(rj)
		s.Equal(j, rj)
	}

	j := job.NewShellJob("true", "")
	s.NoError(s.queue.Put(j))
	rj, ok := s.queue.Get(j.ID())
	s.Nil(rj)
	s.False(ok)
}
