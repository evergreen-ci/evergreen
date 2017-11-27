package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type HostConnectorSuite struct {
	ctx   Connector
	setup func(*HostConnectorSuite)
	suite.Suite
}

const testUser = "user1"

func TestHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestHostConnectorSuite")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(host.Collection))
		host1 := &host.Host{
			Id:             "host1",
			StartedBy:      testUser,
			Status:         evergreen.HostRunning,
			ExpirationTime: time.Now().Add(time.Hour),
		}
		host2 := &host.Host{
			Id:             "host2",
			StartedBy:      "user2",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
		}
		host3 := &host.Host{
			Id:             "host3",
			StartedBy:      "user3",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
		}
		host4 := &host.Host{
			Id:             "host4",
			StartedBy:      "user4",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
		}

		assert.NoError(t, host1.Insert())
		assert.NoError(t, host2.Insert())
		assert.NoError(t, host3.Insert())
		assert.NoError(t, host4.Insert())
	}

	suite.Run(t, s)
}

func TestMockHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.setup = func(s *HostConnectorSuite) {
		s.ctx = &MockConnector{MockHostConnector: MockHostConnector{
			CachedHosts: []host.Host{
				{Id: "host1", StartedBy: testUser, Status: evergreen.HostRunning, ExpirationTime: time.Now().Add(time.Hour)},
				{Id: "host2", StartedBy: "user2", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
				{Id: "host3", StartedBy: "user3", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
				{Id: "host4", StartedBy: "user4", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)}},
		}}

	}
	suite.Run(t, s)
}

func (s *HostConnectorSuite) SetupTest() {
	s.NotNil(s.setup)
	s.setup(s)
}

func (s *HostConnectorSuite) TearDownSuite() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	if session != nil {
		err := session.DB(testConfig.Database.DB).DropDatabase()
		if err != nil {
			panic(err)
		}
	}
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
	h, ok := s.ctx.FindHostById("host5")
	s.Error(ok)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindByUser() {
	hosts, err := s.ctx.FindHostsById("", "", testUser, 100, 1)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		s.Equal(testUser, h.StartedBy)
	}
}

func (s *HostConnectorSuite) TestStatusFiltering() {
	hosts, err := s.ctx.FindHostsById("", "", "", 100, 1)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		statusFound := false
		for _, status := range evergreen.UphostStatus {
			if h.Status == status {
				statusFound = true
			}
		}
		s.True(statusFound)
	}
}

func (s *HostConnectorSuite) TestLimitAndSort() {
	hosts, err := s.ctx.FindHostsById("", evergreen.HostTerminated, "", 2, 1)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(2, len(hosts))
	s.Equal("host2", hosts[0].Id)
	s.Equal("host3", hosts[1].Id)

	hosts, err = s.ctx.FindHostsById("host4", evergreen.HostTerminated, "", 2, -1)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(2, len(hosts))
	s.Equal("host4", hosts[0].Id)
	s.Equal("host3", hosts[1].Id)
}

func (s *HostConnectorSuite) TestSpawnHost() {
	testDistroID := util.RandomString()
	const testPublicKey = "ssh-rsa 1234567890abcdef"
	const testPublicKeyName = "testPubKey"
	const testUserID = "TestSpawnHostUser"
	const testUserAPIKey = "testApiKey"

	testutil.ConfigureIntegrationTest(s.T(), testConfig, "TestSpawnHost")
	distro := &distro.Distro{
		Id:           testDistroID,
		SpawnAllowed: true,
	}
	s.NoError(distro.Insert())
	testUser := &user.DBUser{
		Id:     testUserID,
		APIKey: testUserAPIKey,
	}
	testUser.PubKeys = append(testUser.PubKeys, user.PubKey{
		Name: testPublicKeyName,
		Key:  testPublicKey,
	})
	s.NoError(testUser.Insert())

	//note this is the real DB host connector, not the mock
	intentHost, err := (&DBHostConnector{}).NewIntentHost(testDistroID, testPublicKeyName, "", "", testUser)
	s.NotNil(intentHost)
	s.NoError(err)
	foundHost, err := host.FindOne(host.ById(intentHost.Id))
	s.NotNil(foundHost)
	s.NoError(err)
	s.True(foundHost.UserHost)
	s.Equal(testUserID, foundHost.StartedBy)
}

func (s *HostConnectorSuite) TestSetHostStatus() {
	h, err := s.ctx.FindHostById("host1")
	s.NoError(err)
	s.NoError(s.ctx.SetHostStatus(h, evergreen.HostTerminated))

	for i := 1; i < 5; i++ {
		h, err := s.ctx.FindHostById(fmt.Sprintf("host%d", i))
		s.NoError(err)
		s.Equal(evergreen.HostTerminated, h.Status)
	}
}

func (s *HostConnectorSuite) TestExtendHostExpiration() {
	h, err := s.ctx.FindHostById("host1")
	expectedTime := h.ExpirationTime.Add(5 * time.Hour)
	s.NoError(err)
	s.NoError(s.ctx.ExtendHostExpiration(h, 5))

	hCheck, err := s.ctx.FindHostById("host1")
	s.Equal(expectedTime, hCheck.ExpirationTime)
	s.NoError(err)
}
