package pool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/suite"
)

type AbortablePoolSuite struct {
	pool           amboy.AbortableRunner
	queue          *QueueTester
	setupPool      func()
	setupQueuePool func()
	setJob         func(string, context.CancelFunc)
	runJob         func(context.Context, amboy.Job)
	suite.Suite
}

type cancelFuncCounter struct {
	mu      sync.Mutex
	counter int
}

func (f *cancelFuncCounter) Canceler() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter++
}

func TestAbortablePoolSuite(t *testing.T) {
	s := new(AbortablePoolSuite)
	s.setupPool = func() {
		s.pool = &abortablePool{
			jobs: make(map[string]context.CancelFunc),
		}
	}

	s.setJob = func(id string, cancel context.CancelFunc) {
		s.pool.(*abortablePool).jobs[id] = cancel
	}

	s.setupQueuePool = func() {
		s.pool.(*abortablePool).queue = s.queue
	}

	s.runJob = func(ctx context.Context, j amboy.Job) {
		s.pool.(*abortablePool).runJob(ctx, j)
	}

	suite.Run(t, s)
}

func TestRateLimitedAbortablePoolSuite(t *testing.T) {
	s := new(AbortablePoolSuite)
	s.setupPool = func() {
		s.pool = &ewmaRateLimiting{
			jobs:   make(map[string]context.CancelFunc),
			period: time.Second,
			target: 1000,
			size:   2,
			ewma:   ewma.NewMovingAverage(0.06),
		}
	}

	s.setJob = func(id string, cancel context.CancelFunc) {
		s.pool.(*ewmaRateLimiting).jobs[id] = cancel
	}

	s.setupQueuePool = func() {
		s.pool.(*ewmaRateLimiting).queue = s.queue
	}

	s.runJob = func(ctx context.Context, j amboy.Job) {
		_ = s.pool.(*ewmaRateLimiting).runJob(ctx, j)
	}

	suite.Run(t, s)
}

func (s *AbortablePoolSuite) SetupTest() {
	s.setupPool()
	s.queue = &QueueTester{
		pool:      s.pool,
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}
	s.setupQueuePool()
}

func (s *AbortablePoolSuite) TestImplementationCompliance() {
	s.Implements((*amboy.Runner)(nil), s.pool)
	s.Implements((*amboy.AbortableRunner)(nil), s.pool)

	constructed := NewAbortablePool(2, s.queue)
	s.NotNil(constructed)
	s.Implements((*amboy.Runner)(nil), constructed)
	s.Implements((*amboy.AbortableRunner)(nil), constructed)
}

func (s *AbortablePoolSuite) TestConstructorUnflappability() {
	constructed := NewAbortablePool(-1, nil)
	s.NotNil(constructed)
}

func (s *AbortablePoolSuite) TestCloserCancelsFuncs() {
	closer := &cancelFuncCounter{}
	s.Equal(0, closer.counter)
	s.Len(s.pool.RunningJobs(), 0)

	s.setJob("id", closer.Canceler)
	s.Equal(0, closer.counter)
	s.Len(s.pool.RunningJobs(), 1)

	s.pool.Close(context.TODO())
	s.Equal(1, closer.counter)
	s.Len(s.pool.RunningJobs(), 0)
}

func (s *AbortablePoolSuite) TestSingleAborterErrorsForUnknownJob() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Error(s.pool.Abort(ctx, "foo"))
	s.Error(s.pool.Abort(ctx, "DOES NOT EXIST"))
}

func (s *AbortablePoolSuite) TestSingleAborterCancelsJob() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	closer := &cancelFuncCounter{}
	s.Equal(0, closer.counter)
	s.setJob("id", closer.Canceler)
	s.Len(s.pool.RunningJobs(), 1)
	s.Equal(0, closer.counter)

	s.Error(s.pool.Abort(ctx, "foo"))
	s.Len(s.pool.RunningJobs(), 1)
	s.Equal(0, closer.counter)

	_ = s.pool.Abort(ctx, "id")
	s.Len(s.pool.RunningJobs(), 0)
	s.Equal(1, closer.counter)
}

func (s *AbortablePoolSuite) TestAbortAllWorks() {
	closers := make([]*cancelFuncCounter, 10)

	s.Len(s.pool.RunningJobs(), 0)
	count := 0
	seen := 0
	for idx := range closers {
		closers[idx] = &cancelFuncCounter{}

		seen++
		count += closers[idx].counter
		s.setJob(fmt.Sprint(idx), closers[idx].Canceler)
	}
	s.Equal(10, seen)
	s.Equal(0, count)

	s.Len(s.pool.RunningJobs(), 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.pool.AbortAll(ctx)
	s.Len(s.pool.RunningJobs(), 0)
	seen = 0
	for _, c := range closers {
		count += c.counter
		seen++
	}
	s.Equal(10, seen)
	s.Equal(10, count)
}

func (s *AbortablePoolSuite) TestIntrospectionMethods() {
	closers := make([]*cancelFuncCounter, 10)

	s.Len(s.pool.RunningJobs(), 0)
	count := 0
	seen := 0
	for idx := range closers {
		closers[idx] = &cancelFuncCounter{}

		seen++
		count += closers[idx].counter
		s.setJob(fmt.Sprint(idx), closers[idx].Canceler)
	}

	for idx := range closers {
		s.True(s.pool.IsRunning(fmt.Sprint(idx)))
		s.False(s.pool.IsRunning(fmt.Sprintf("%d-NOT_RUNNING", idx)))
	}

	jobNames := s.pool.RunningJobs()
	s.Len(jobNames, 10)
	sort.Strings(jobNames)

	for idx := range closers {
		s.Equal(jobNames[idx], fmt.Sprint(idx))
	}
}

func (s *AbortablePoolSuite) TestAbortableRunJob() {
	s.Len(s.pool.RunningJobs(), 0)

	j := &jobThatPanics{}
	ctx := context.Background()
	s.Panics(func() { s.runJob(ctx, j) })
	s.Len(s.pool.RunningJobs(), 0)
}
