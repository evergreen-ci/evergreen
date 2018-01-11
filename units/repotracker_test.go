package units

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/admin"
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

func (s *repotrackerJobSuite) TestJob() {
	j := NewRepotrackerJob("1", "evergreen-ci", "evergreen").(*repotrackerJob)
	s.Equal("evergreen-ci", j.Owner)
	s.Equal("evergreen", j.Repo)
	s.Equal("repotracker:evergreen-ci/evergreen-1", j.ID())
	j.Run()
	s.Error(j.Error())
	s.Equal("not found", j.Error().Error())
	s.True(j.Status().Completed)
}

func (s *repotrackerJobSuite) TestRunFailsInDegradedMode() {
	s.NoError(db.Clear(admin.Collection))

	flags := admin.ServiceFlags{
		RepotrackerPushEventDisabled: true,
	}
	s.NoError(admin.SetServiceFlags(flags))

	job := NewRepotrackerJob("1", "evergreen-ci", "evergreen")
	job.Run()

	s.Error(job.Error())
	s.Contains(job.Error().Error(), "github push events triggering repotracker is disabled")
	s.NoError(db.Clear(admin.Collection))
}
