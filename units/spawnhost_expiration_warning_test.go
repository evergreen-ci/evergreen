package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type spawnHostExpirationSuite struct {
	j *spawnhostExpirationWarningsJob
	suite.Suite
	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context
}

func TestSpawnHostExpiration(t *testing.T) {
	s := new(spawnHostExpirationSuite)
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)
	suite.Run(t, s)
}

func (s *spawnHostExpirationSuite) SetupSuite() {
	s.j = makeSpawnhostExpirationWarningsJob()
}

func (s *spawnHostExpirationSuite) TearDownSuite() {
	s.cancel()
}

func (s *spawnHostExpirationSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	s.NoError(db.ClearCollections(event.EventCollection, host.Collection, alertrecord.Collection))
	now := time.Now()
	h1 := host.Host{
		Id:             "h1",
		ExpirationTime: now.Add(13 * time.Hour),
	}
	h2 := host.Host{ // should get a 12 hr warning
		Id:             "h2",
		ExpirationTime: now.Add(9 * time.Hour),
	}
	h3 := host.Host{ // should get a 12 and 2 hr warning
		Id:             "h3",
		ExpirationTime: now.Add(1 * time.Hour),
	}
	s.NoError(h1.Insert(s.ctx))
	s.NoError(h2.Insert(s.ctx))
	s.NoError(h3.Insert(s.ctx))
}

func (s *spawnHostExpirationSuite) TestAlerts() {
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 3)
}

func (s *spawnHostExpirationSuite) TestCanceledJob() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	s.j.Run(ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 0)
}
