package queue

// the smoke tests cover most operations of a queue under a number of
// different situations. these tests just fill in the gaps. and help us ensure
// consistent behavior of this implementation

import (
	"context"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ShuffledQueueSuite struct {
	require *require.Assertions
	queue   *shuffledLocal
	suite.Suite
}

func TestShuffledQueueSuite(t *testing.T) {
	suite.Run(t, new(ShuffledQueueSuite))
}

func (s *ShuffledQueueSuite) SetupSuite() {
	s.require = s.Require()
}

func (s *ShuffledQueueSuite) SetupTest() {
	s.queue = &shuffledLocal{
		capacity: defaultLocalQueueCapcity,
		scopes:   NewLocalScopeManager(),
	}
	s.queue.dispatcher = NewDispatcher(s.queue)

}

func (s *ShuffledQueueSuite) TestCannotStartQueueWithNilRunner() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// make sure that unstarted queues without runners will error
	// if you attempt to set them
	s.False(s.queue.Started())
	s.Nil(s.queue.runner)
	s.Error(s.queue.Start(ctx))
	s.False(s.queue.Started())

	// now validate the inverse
	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NotNil(s.queue.runner)
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())
}

func (s *ShuffledQueueSuite) TestPutFailsWithUnstartedQueue() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.False(s.queue.Started())
	s.Error(s.queue.Put(ctx, job.NewShellJob("echo 1", "")))

	// now validate the inverse
	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NoError(s.queue.Start(ctx))
	s.True(s.queue.Started())

	s.NoError(s.queue.Put(ctx, job.NewShellJob("echo 1", "")))
}

func (s *ShuffledQueueSuite) TestPutFailsIfJobIsTracked() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NoError(s.queue.Start(ctx))

	j := job.NewShellJob("echo 1", "")

	// first, attempt works fine
	s.NoError(s.queue.Put(ctx, j))

	// afterwords, attempts should fail
	for i := 0; i < 10; i++ {
		s.Error(s.queue.Put(ctx, j))
	}
}

func (s *ShuffledQueueSuite) TestStatsShouldReturnNilObjectifQueueIsNotRunning() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.False(s.queue.Started())
	for i := 0; i < 20; i++ {
		s.Equal(amboy.QueueStats{}, s.queue.Stats(ctx))
	}
}

func (s *ShuffledQueueSuite) TestSetRunnerReturnsErrorIfRunnerHasStarted() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NoError(s.queue.Start(ctx))
	origRunner := s.queue.Runner()

	s.Error(s.queue.SetRunner(pool.NewSingle()))

	s.Exactly(origRunner, s.queue.Runner())
}

func (s *ShuffledQueueSuite) TestGetMethodRetrieves() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("true", "")

	jReturn, ok := s.queue.Get(ctx, j.ID())
	s.False(ok)
	s.Nil(jReturn)

	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NoError(s.queue.Start(ctx))

	jReturn, ok = s.queue.Get(ctx, j.ID())
	s.False(ok)
	s.Nil(jReturn)

	s.NoError(s.queue.Put(ctx, j))

	jReturn, ok = s.queue.Get(ctx, j.ID())
	s.True(ok)
	s.Exactly(jReturn, j)
	amboy.Wait(ctx, s.queue)

	jReturn, ok = s.queue.Get(ctx, j.ID())
	s.True(ok)
	s.Exactly(jReturn, j)
}

func (s *ShuffledQueueSuite) TestResultsOperationReturnsEmptyChannelIfQueueIsNotStarted() {
	s.False(s.queue.Started())
	count := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for range s.queue.Results(ctx) {
		count++
	}

	s.Equal(0, count)
}

func (s *ShuffledQueueSuite) TestCompleteReturnsIfContextisCanceled() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.queue.SetRunner(pool.NewSingle()))
	s.NoError(s.queue.Start(ctx))

	ctx2, cancel2 := context.WithCancel(ctx)
	j := job.NewShellJob("false", "")
	cancel2()
	s.queue.Complete(ctx2, j)
	stat := s.queue.Stats(ctx)
	s.Equal(0, stat.Completed)
}
