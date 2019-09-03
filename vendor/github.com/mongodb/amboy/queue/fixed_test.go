package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// LocalLimitedSizeQueue suite tests the queue implementation that
// uses the CappedResultStorage. These tests exercise the aspects of
// this queue implementation that are not covered by the tests of the
// storage object *or* exercised by the general queue functionality
// tests, which test all implementations.
type LimitedSizeQueueSuite struct {
	queue       *limitedSizeLocal
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
	s.queue = NewLocalLimitedSize(s.numWorkers, s.numCapacity).(*limitedSizeLocal)
}

func (s *LimitedSizeQueueSuite) TestBufferForPendingWorkEqualToCapacityForResults() {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	s.False(s.queue.Started())
	s.queue.Runner().Close(ctx)
	s.Nil(s.queue.channel)
	s.Error(s.queue.Put(ctx, job.NewShellJob("sleep 10", "")))

	s.NoError(s.queue.Start(ctx))
	s.require.True(s.queue.Started())
	for i := 0; i < 100*s.numCapacity*s.numWorkers; i++ {
		var outcome bool
		err := s.queue.Put(ctx, job.NewShellJob("sleep 10", ""))
		if i < s.numWorkers+s.numCapacity {
			outcome = s.NoError(err, "idx=%d stat=%+v", i, s.queue.Stats(ctx))
		} else {
			outcome = s.Error(err, "idx=%d", i)
		}

		if !outcome {
			break
		}
	}

	s.Len(s.queue.channel, s.numCapacity)
	s.True(len(s.queue.storage) == s.numCapacity+s.numWorkers, fmt.Sprintf("storage=%d", len(s.queue.storage)))
	s.Error(s.queue.Put(ctx, job.NewShellJob("sleep 10", "")))
	s.True(len(s.queue.storage) == s.numCapacity+s.numWorkers, fmt.Sprintf("storage=%d", len(s.queue.storage)))
	s.Len(s.queue.channel, s.numCapacity)
}

func (s *LimitedSizeQueueSuite) TestCallingStartMultipleTimesDoesNotImpactState() {
	s.False(s.queue.Started())
	s.Nil(s.queue.channel)
	ctx := context.Background()
	s.NoError(s.queue.Start(ctx))

	s.NotNil(s.queue.channel)
	for i := 0; i < 100; i++ {
		s.Error(s.queue.Start(ctx))
	}

	s.NotNil(s.queue.channel)
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
