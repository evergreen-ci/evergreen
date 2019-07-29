package queue

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/suite"
)

type LockManagerSuite struct {
	lm         *lockManager
	driver     *priorityDriver
	testCancel context.CancelFunc
	ctx        context.Context
	suite.Suite
}

func TestLockManagerSuite(t *testing.T) {
	s := &LockManagerSuite{}
	suite.Run(t, s)
}

func (s *LockManagerSuite) SetupSuite() {
	s.driver = NewPriorityDriver().(*priorityDriver)
}

func (s *LockManagerSuite) SetupTest() {
	s.ctx, s.testCancel = context.WithCancel(context.Background())
	s.lm = newLockManager("test", s.driver)
	s.lm.timeout = 100 * time.Millisecond
	s.lm.start(s.ctx)
}

func (s *LockManagerSuite) TearDownTest() {
	s.testCancel()
}

func (s *LockManagerSuite) TearDownSuite() {
	s.driver.Close()
}

func (s *LockManagerSuite) TestCannotLockOrUnlockANilJob() {
	s.Error(s.lm.Lock(s.ctx, nil))
	s.Error(s.lm.Unlock(s.ctx, nil))
	var j amboy.Job
	s.Error(s.lm.Lock(s.ctx, j))
	s.Error(s.lm.Unlock(s.ctx, j))
}

func (s *LockManagerSuite) TestSuccessiveAttemptsToTakeALockAreErrors() {
	j := job.NewShellJob("echo hi", "")
	s.NoError(s.driver.Put(s.ctx, j))

	s.NoError(s.lm.Lock(s.ctx, j))

	for i := 0; i < 10; i++ {
		s.Error(s.lm.Lock(s.ctx, j))
	}
}

func (s *LockManagerSuite) TestLockAndUnlockCylcesWorkForOneJob() {
	j := job.NewShellJob("echo hello", "")
	s.NoError(s.driver.Put(s.ctx, j))

	for i := 0; i < 10; i++ {
		s.NoError(s.lm.Lock(s.ctx, j))
		s.NoError(s.lm.Unlock(s.ctx, j))
	}
}

func (s *LockManagerSuite) TestLocksArePerJob() {
	jone := job.NewShellJob("echo hi", "")
	jtwo := job.NewShellJob("echo world", "")
	s.NoError(s.driver.Put(s.ctx, jone))
	s.NoError(s.driver.Put(s.ctx, jtwo))
	s.NotEqual(jone.ID(), jtwo.ID())

	s.NoError(s.lm.Lock(s.ctx, jone))
	s.NoError(s.lm.Lock(s.ctx, jtwo))
}

func (s *LockManagerSuite) TestLockReachesTimeout() {
	j := job.NewShellJob("echo hello", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.NoError(s.driver.Put(s.ctx, j))

	// pass it with a canceled
	// context to disable the pinger
	s.NoError(s.lm.Lock(ctx, j))
	s.Error(s.lm.Lock(s.ctx, j))
	time.Sleep(s.lm.timeout * 10)
	s.NoError(s.lm.Lock(s.ctx, j))
}

func (s *LockManagerSuite) TestPanicJobIsUnlocked() {
	j := &jobThatPanics{
		sleep: time.Second,
	}
	j.SetID("foo")

	ctx, cancel := context.WithCancel(context.Background())

	lastMod := j.Status().ModificationCount
	s.Equal(0, lastMod)
	s.NoError(s.driver.Put(ctx, j))
	lastMod = j.Status().ModificationCount
	s.Equal(0, lastMod)

	s.NoError(s.lm.Lock(ctx, j), "%+v", j)

	for i := 0; i < 10; i++ {
		s.Error(s.lm.Lock(ctx, j), "idx=%d => %+v", i, j)
	}

	time.Sleep(s.lm.timeout)

	lastMod = j.Status().ModificationCount
	s.True(lastMod >= 1)

	s.False(j.Status().Completed)
	cancel()
	time.Sleep(10 * s.lm.timeout)

	s.NoError(s.lm.Lock(ctx, j), "%+v", j)
	lastMod = j.Status().ModificationCount

	time.Sleep(200 * time.Millisecond)
	s.Equal(lastMod, j.Status().ModificationCount)
	s.False(j.Status().Completed)
	time.Sleep(200 * time.Millisecond)

	s.NoError(s.lm.Lock(s.ctx, j))

	for i := 0; i < 10; i++ {
		s.Require().Error(s.lm.Lock(s.ctx, j), "idx=%d => %+v", i, j)
	}
}
