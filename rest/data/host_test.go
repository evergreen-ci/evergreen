package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type HostConnectorSuite struct {
	ctx Connector
	suite.Suite
}

func TestHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestHostConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	host1 := &host.Host{Id: "host1"}
	host2 := &host.Host{Id: "host2"}

	assert.NoError(t, host1.Insert())
	assert.NoError(t, host2.Insert())
	suite.Run(t, s)
}

func TestMockHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.ctx = &MockConnector{MockHostConnector: MockHostConnector{
		CachedHosts: []host.Host{{Id: "host1"}, {Id: "host2"}},
	}}
	suite.Run(t, s)
}

func (s *HostConnectorSuite) TestFindByIdFirst() {
	h, ok := s.ctx.FindHostById("host1")
	s.NoError(ok)
	s.NotNil(h)
	s.Equal("host1", h.Id)
}

func (s *HostConnectorSuite) TestFindByIdLast() {
	h, ok := s.ctx.FindHostById("host2")
	s.NoError(ok)
	s.NotNil(h)
	s.Equal("host2", h.Id)
}

func (s *HostConnectorSuite) TestFindByIdFail() {
	h, ok := s.ctx.FindHostById("host3")
	s.Error(ok)
	s.Nil(h)
}
