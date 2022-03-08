package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type cliUpdateConnectorSuite struct {
	suite.Suite
	ctx     CLIUpdateConnector
	setup   func()
	degrade func()
}

func TestUpdateConnector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	s := &cliUpdateConnectorSuite{}
	s.setup = func() {
		s.NoError(db.ClearCollections(evergreen.ConfigCollection))
	}
	s.degrade = func() {
		flags := evergreen.ServiceFlags{
			CLIUpdatesDisabled: true,
		}
		s.NoError(evergreen.SetServiceFlags(flags))
	}
	suite.Run(t, s)
}

func (s *cliUpdateConnectorSuite) SetupSuite() {
	s.setup()
}

func (s *cliUpdateConnectorSuite) Test() {
	v, err := s.ctx.GetCLIUpdate()
	s.Require().NoError(err)
	s.Require().NotNil(v)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}

func (s *cliUpdateConnectorSuite) TestDegradedMode() {
	s.degrade()
	v, err := s.ctx.GetCLIUpdate()
	s.NoError(err)
	s.Require().NotNil(v)
	s.True(v.IgnoreUpdate)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}
