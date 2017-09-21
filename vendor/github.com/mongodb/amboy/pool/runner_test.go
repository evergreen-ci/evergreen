package pool

import (
	"testing"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type RunnerSuite struct {
	pool    amboy.Runner
	queue   *QueueTester
	factory func() amboy.Runner

	suite.Suite
}

func TestLocalRunnerBaseSuite(t *testing.T) {
	s := new(RunnerSuite)
	s.factory = func() amboy.Runner { return new(localWorkers) }

	suite.Run(t, s)
}

func TestSingleRunnerBaseSuite(t *testing.T) {
	s := new(RunnerSuite)
	s.factory = func() amboy.Runner { return new(single) }

	suite.Run(t, s)
}

func TestSimpleRateLimitedSuite(t *testing.T) {
	s := new(RunnerSuite)
	s.factory = func() amboy.Runner {
		return &simpleRateLimited{
			size:     2,
			interval: time.Second,
		}
	}

	suite.Run(t, s)
}

func TestAverageRateLimitedSuite(t *testing.T) {
	s := new(RunnerSuite)
	s.factory = func() amboy.Runner {
		return &ewmaRateLimiting{
			size:   2,
			period: time.Second,
			target: 5,
			ewma:   ewma.NewMovingAverage(),
		}
	}

	suite.Run(t, s)
}

func TestGroupWorker(t *testing.T) {
	s := new(RunnerSuite)
	s.factory = func() amboy.Runner {
		return &Group{
			size: 2,
		}
	}

	suite.Run(t, s)
}

func (s *RunnerSuite) SetupTest() {
	s.pool = s.factory()
	s.queue = &QueueTester{
		pool:      s.pool,
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}

}

func (s *RunnerSuite) TestNotStartedByDefault() {
	s.False(s.pool.Started())
}

func (s *RunnerSuite) TestQuequeIsMutable() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// it's an unconfigured runner without a queue, it should always error
	s.Error(s.pool.Start(ctx))

	// this should start the queue
	s.NoError(s.pool.SetQueue(s.queue))

	// it's cool to start the runner
	s.NoError(s.pool.Start(ctx))

	// once the runner starts you can't add pools
	s.Error(s.pool.SetQueue(s.queue))

	// subsequent calls to start should noop
	s.NoError(s.pool.Start(ctx))
}

func (s *RunnerSuite) TestCloseDoesNotAffectState() {
	s.False(s.pool.Started())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.pool.Close()

	s.False(s.pool.Started())
	s.NoError(s.pool.SetQueue(s.queue))
	s.NoError(s.pool.Start(ctx))

	s.pool.Close()

	s.True(s.pool.Started())
}
