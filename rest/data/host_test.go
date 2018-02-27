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
	"github.com/stretchr/testify/suite"
)

type HostConnectorSuite struct {
	ctx   Connector
	setup func(*HostConnectorSuite)
	suite.Suite
}

const testUser = "user1"

func TestHostConnectorSuite(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestHostConnectorSuite")
	s := new(HostConnectorSuite)
	s.ctx = &DBConnector{}
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(user.Collection, host.Collection))
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

		s.NoError(host1.Insert())
		s.NoError(host2.Insert())
		s.NoError(host3.Insert())
		s.NoError(host4.Insert())

		users := []string{testUser, "user2", "user3", "user4", "root"}

		for _, id := range users {
			user := &user.DBUser{
				Id: id,
			}
			s.NoError(user.Insert())
		}

		s.ctx.SetSuperUsers([]string{"root"})
	}

	suite.Run(t, s)
}

func TestMockHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.setup = func(s *HostConnectorSuite) {
		s.ctx = &MockConnector{
			MockHostConnector: MockHostConnector{
				CachedHosts: []host.Host{
					{Id: "host1", StartedBy: testUser, Status: evergreen.HostRunning, ExpirationTime: time.Now().Add(time.Hour)},
					{Id: "host2", StartedBy: "user2", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
					{Id: "host3", StartedBy: "user3", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
					{Id: "host4", StartedBy: "user4", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)}},
			},
			MockUserConnector: MockUserConnector{
				CachedUsers: map[string]*user.DBUser{
					testUser: {
						Id: testUser,
					},
					"user2": {
						Id: "user2",
					},
					"user3": {
						Id: "user3",
					},
					"user4": {
						Id: "user4",
					},
					"root": {
						Id: "root",
					},
				},
			},
		}
		s.ctx.SetSuperUsers([]string{"root"})
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
	intentHost, err := (&DBHostConnector{}).NewIntentHost(testDistroID, testPublicKeyName, "", testUser)
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
	s.NoError(s.ctx.SetHostStatus(h, evergreen.HostTerminated, evergreen.User))

	for i := 1; i < 5; i++ {
		h, err := s.ctx.FindHostById(fmt.Sprintf("host%d", i))
		s.NoError(err)
		s.Equal(evergreen.HostTerminated, h.Status)
	}
}

func (s *HostConnectorSuite) TestExtendHostExpiration() {
	h, err := s.ctx.FindHostById("host1")
	s.NoError(err)
	expectedTime := h.ExpirationTime.Add(5 * time.Hour)
	s.NoError(s.ctx.SetHostExpirationTime(h, expectedTime))

	hCheck, err := s.ctx.FindHostById("host1")
	s.Equal(expectedTime, hCheck.ExpirationTime)
	s.NoError(err)
}

func (s *HostConnectorSuite) TestFindHostByIdWithOwner() {
	u, err := s.ctx.FindUserById(testUser)
	s.NoError(err)

	h, err := s.ctx.FindHostByIdWithOwner("host1", u)
	s.NoError(err)
	s.NotNil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdFailsWithWrongUser() {
	u, err := s.ctx.FindUserById(testUser)
	s.NoError(err)
	s.NotNil(u)

	h, err := s.ctx.FindHostByIdWithOwner("host2", u)
	s.Error(err)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdWithSuperUser() {
	u, err := s.ctx.FindUserById("root")
	s.NoError(err)

	h, err := s.ctx.FindHostByIdWithOwner("host2", u)
	s.NoError(err)
	s.NotNil(h)
}
