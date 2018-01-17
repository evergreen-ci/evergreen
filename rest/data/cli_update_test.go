package data

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
		s.NoError(evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))

		s.NoError(db.ClearCollections(admin.Collection))
	}
	s.degrade = func() {
		flags := admin.ServiceFlags{
			CLIUpdatesDisabled: true,
		}
		s.NoError(admin.SetServiceFlags(flags))
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
	evergreen.ResetEnvironment()
	s.setup()
}
func (s *cliUpdateConnectorSuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *cliUpdateConnectorSuite) Test() {
	v, err := s.ctx.GetCLIUpdate()
	s.NoError(err)
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
