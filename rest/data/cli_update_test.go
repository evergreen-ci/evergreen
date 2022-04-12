package data

import (
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
	v, err := GetCLIUpdate()
	s.Require().NoError(err)
	s.Require().NotNil(v)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}

func (s *cliUpdateConnectorSuite) TestDegradedMode() {
	s.degrade()
	v, err := GetCLIUpdate()
	s.NoError(err)
	s.Require().NotNil(v)
	s.True(v.IgnoreUpdate)
	s.NotEmpty(v.ClientConfig.LatestRevision)
}
