package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

type spawnHostExpirationSuite struct {
	j *spawnhostExpirationWarningsJob
	suite.Suite
}

func TestSpawnHostExpiration(t *testing.T) {
	suite.Run(t, new(spawnHostExpirationSuite))
}

func (s *spawnHostExpirationSuite) SetupSuite() {
	s.j = makeSpawnhostExpirationWarningsJob()
}

func (s *spawnHostExpirationSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, host.Collection, alertrecord.Collection))
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
	s.NoError(h1.Insert())
	s.NoError(h2.Insert())
	s.NoError(h3.Insert())
}

func (s *spawnHostExpirationSuite) TestAlerts() {
	ctx := context.Background()
	s.j.Run(ctx)
	events, err := event.FindUnprocessedEvents()
	s.NoError(err)
	s.Len(events, 3)
}

func (s *spawnHostExpirationSuite) TestCanceledJob() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.j.Run(ctx)
	events, err := event.FindUnprocessedEvents()
	s.NoError(err)
	s.Len(events, 0)
}
