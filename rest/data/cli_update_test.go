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
	ctx     Connector
	setup   func()
	degrade func()
	cancel  func()
}

func TestUpdateConnector(t *testing.T) {
	s := &cliUpdateConnectorSuite{
		ctx: &DBConnector{},
	}
	s.setup = func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel

		testutil.SetGlobalEnvironment(ctx, t)

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

func TestMockUpdateConnector(t *testing.T) {
	s := &cliUpdateConnectorSuite{
		ctx: &MockConnector{},
	}
	s.setup = func() {
	}
	s.degrade = func() {
		s.ctx.(*MockConnector).MockCLIUpdateConnector.degradedModeOn = true
	}
	suite.Run(t, s)
}

func (s *cliUpdateConnectorSuite) SetupSuite() {
	s.setup()
}
func (s *cliUpdateConnectorSuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}
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
