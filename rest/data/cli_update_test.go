package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
)

type cliUpdateConnectorSuite struct {
	suite.Suite
	setup   func()
	degrade func()
}

func TestUpdateConnector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &cliUpdateConnectorSuite{}
	s.setup = func() {
		s.NoError(db.ClearCollections(evergreen.ConfigCollection))
	}
	s.degrade = func() {
		flags := evergreen.ServiceFlags{
			CLIUpdatesDisabled: true,
		}
		s.NoError(evergreen.SetServiceFlags(ctx, flags))
	}
	suite.Run(t, s)
}

func (s *cliUpdateConnectorSuite) SetupSuite() {
	s.setup()
}

func (s *cliUpdateConnectorSuite) Test() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	v, err := GetCLIUpdate(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(v)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}

func (s *cliUpdateConnectorSuite) TestDegradedMode() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.degrade()
	v, err := GetCLIUpdate(ctx)
	s.NoError(err)
	s.Require().NotNil(v)
	s.True(v.IgnoreUpdate)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}
