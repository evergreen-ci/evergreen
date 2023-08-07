package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type repotrackerJobSuite struct {
	suite.Suite
	suiteCtx context.Context
	cancel   context.CancelFunc
}

func TestRepotrackerJob(t *testing.T) {
	s := new(repotrackerJobSuite)
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)
	suite.Run(t, s)
}

func (s *repotrackerJobSuite) TearDownSuite() {
	s.cancel()
}

func (s *repotrackerJobSuite) SetupTest() {
	testutil.TestSpan(s.suiteCtx, s.T())
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
}

func (s *repotrackerJobSuite) TearDownTest() {
	s.NoError(db.ClearCollections(evergreen.ConfigCollection))
}

func (s *repotrackerJobSuite) TestJob() {
	j := NewRepotrackerJob("1", "mci").(*repotrackerJob)
	s.Equal("mci", j.ProjectID)
	s.Equal("repotracker:1:mci", j.ID())
	j.Run(context.Background())
	s.Error(j.Error())
	s.Contains(j.Error().Error(), "project ref 'mci' not found")
	s.True(j.Status().Completed)
}

func (s *repotrackerJobSuite) TestRunFailsInDegradedMode() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flags := evergreen.ServiceFlags{
		RepotrackerDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(ctx, flags))

	job := NewRepotrackerJob("1", "mci")
	job.Run(context.Background())

	s.Error(job.Error())
	s.Contains(job.Error().Error(), "repotracker is disabled")
}
