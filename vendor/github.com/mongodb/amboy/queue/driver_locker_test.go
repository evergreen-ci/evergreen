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
	lm          *lockManager
	driver      *priorityDriver
	testCancel  context.CancelFunc
	suiteCancel context.CancelFunc
	ctx         context.Context
	suite.Suite
}

func TestLockManagerSuite(t *testing.T) {
	s := &LockManagerSuite{}
	suite.Run(t, s)
}

func (s *LockManagerSuite) SetupSuite() {
	var ctx context.Context
	s.driver = NewPriorityDriver().(*priorityDriver)
	ctx, s.suiteCancel = context.WithCancel(context.Background())
	s.Require().NoError(s.driver.Open(ctx))
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
	s.suiteCancel()
	s.driver.Close()
}

func (s *LockManagerSuite) TestCannotLockOrUnlockANilJob() {
	s.Error(s.lm.Lock(s.ctx, nil))
	s.Error(s.lm.Unlock(nil))
	var j amboy.Job
	s.Error(s.lm.Lock(s.ctx, j))
	s.Error(s.lm.Unlock(j))
}

func (s *LockManagerSuite) TestSuccessiveAttemptsToTakeALockAreErrors() {
	j := job.NewShellJob("echo hi", "")
	s.NoError(s.driver.Put(j))

	s.NoError(s.lm.Lock(s.ctx, j))

	for i := 0; i < 10; i++ {
		s.Error(s.lm.Lock(s.ctx, j))
	}
}

func (s *LockManagerSuite) TestLockAndUnlockCylcesWorkForOneJob() {
	j := job.NewShellJob("echo hello", "")
	s.NoError(s.driver.Put(j))

	for i := 0; i < 10; i++ {
		s.NoError(s.lm.Lock(s.ctx, j))
		s.NoError(s.lm.Unlock(j))
	}
}

func (s *LockManagerSuite) TestLocksArePerJob() {
	jone := job.NewShellJob("echo hi", "")
	jtwo := job.NewShellJob("echo world", "")
	s.NoError(s.driver.Put(jone))
	s.NoError(s.driver.Put(jtwo))
	s.NotEqual(jone.ID(), jtwo.ID())

	s.NoError(s.lm.Lock(s.ctx, jone))
	s.NoError(s.lm.Lock(s.ctx, jtwo))
}

func (s *LockManagerSuite) TestLockReachesTimeout() {
	j := job.NewShellJob("echo hello", "")
	s.NoError(s.driver.Put(j))

	s.NoError(s.lm.Lock(s.ctx, j))
	time.Sleep(s.lm.timeout * 3)
	s.NoError(s.lm.Lock(s.ctx, j))
	s.Error(s.lm.Lock(s.ctx, j))
}
