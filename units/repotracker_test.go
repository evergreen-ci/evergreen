package units

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type repotrackerJobSuite struct {
	suite.Suite
	cancel func()
}

func TestRepotrackerJob(t *testing.T) {
	suite.Run(t, new(repotrackerJobSuite))
}

func (s *repotrackerJobSuite) SetupTest() {
	evergreen.ResetEnvironment()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
}

func (s *repotrackerJobSuite) TearDownTest() {
	s.cancel()
	evergreen.ResetEnvironment()
}

func (s *repotrackerJobSuite) TestRepotrackerJob() {
	j := NewRepotrackerJob("1", "evergreen-ci", "evergreen").(*repotrackerJob)
	s.Equal("evergreen-ci", j.Owner)
	s.Equal("evergreen", j.Repo)
	s.Equal("repotracker:evergreen-ci/evergreen-1", j.ID())
	j.Run()
	s.Error(j.Error())
	s.Equal("not found", j.Error().Error())
	s.True(j.Status().Completed)
}
