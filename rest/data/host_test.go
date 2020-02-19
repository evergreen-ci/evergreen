package data

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(user.Collection, host.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
		cmd := map[string]string{
			"create": evergreen.ScopeCollection,
		}
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
		host1 := &host.Host{
			Id:             "host1",
			StartedBy:      testUser,
			Status:         evergreen.HostRunning,
			ExpirationTime: time.Now().Add(time.Hour),
			Secret:         "abcdef",
		}
		host2 := &host.Host{
			Id:        "host2",
			StartedBy: "user2",
			Distro: distro.Distro{
				Id: "distro2",
			},
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
		host5 := &host.Host{
			Id:        "host5",
			StartedBy: evergreen.User,
			Status:    evergreen.HostRunning,
			Distro: distro.Distro{
				Id: "distro5",
			},
		}

		s.NoError(host1.Insert())
		s.NoError(host2.Insert())
		s.NoError(host3.Insert())
		s.NoError(host4.Insert())
		s.NoError(host5.Insert())

		users := []string{testUser, "user2", "user3", "user4"}

		for _, id := range users {
			user := &user.DBUser{
				Id: id,
			}
			s.NoError(user.Insert())
		}
		root := user.DBUser{
			Id:          "root",
			SystemRoles: []string{"root"},
		}
		s.NoError(root.Insert())
		rm := evergreen.GetEnvironment().RoleManager()
		s.NoError(rm.AddScope(gimlet.Scope{
			ID:        "root",
			Resources: []string{"distro2", "distro5"},
			Type:      evergreen.DistroResourceType,
		}))
		s.NoError(rm.UpdateRole(gimlet.Role{
			ID:    "root",
			Scope: "root",
			Permissions: gimlet.Permissions{
				evergreen.PermissionHosts: evergreen.HostsEdit.Value,
			},
		}))
	}

	suite.Run(t, s)
}

func TestMockHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(evergreen.ScopeCollection, evergreen.RoleCollection))
		cmd := map[string]string{
			"create": evergreen.ScopeCollection,
		}
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
		s.ctx = &MockConnector{
			MockHostConnector: MockHostConnector{
				CachedHosts: []host.Host{
					{Id: "host1", StartedBy: testUser, Status: evergreen.HostRunning, ExpirationTime: time.Now().Add(time.Hour), Secret: "abcdef"},
					{Id: "host2", StartedBy: "user2", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour), Distro: distro.Distro{Id: "distro2"}},
					{Id: "host3", StartedBy: "user3", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
					{Id: "host4", StartedBy: "user4", Status: evergreen.HostTerminated, ExpirationTime: time.Now().Add(time.Hour)},
					{Id: "host5", StartedBy: evergreen.User, Status: evergreen.HostRunning, Distro: distro.Distro{Id: "distro5"}}},
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
						Id:          "root",
						SystemRoles: []string{"root"},
					},
				},
			},
		}
		rm := evergreen.GetEnvironment().RoleManager()
		s.NoError(rm.AddScope(gimlet.Scope{
			ID:        "root",
			Resources: []string{"distro2", "distro5"},
			Type:      evergreen.DistroResourceType,
		}))
		s.NoError(rm.UpdateRole(gimlet.Role{
			ID:    "root",
			Scope: "root",
			Permissions: gimlet.Permissions{
				evergreen.PermissionHosts: evergreen.HostsEdit.Value,
			},
		}))
	}
	suite.Run(t, s)
}

func (s *HostConnectorSuite) SetupTest() {
	s.NotNil(s.setup)
	s.setup(s)
}

func (s *HostConnectorSuite) TearDownSuite() {
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
	h, ok := s.ctx.FindHostById("nonexistent")
	s.Error(ok)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindHostsByDistroID() {
	hosts, err := s.ctx.FindHostsByDistroID("distro5")
	s.Require().NoError(err)
	s.Require().Len(hosts, 1)
	s.Equal("host5", hosts[0].Id)
}

func (s *HostConnectorSuite) TestFindByUser() {
	hosts, err := s.ctx.FindHostsById("", "", testUser, 100)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		s.Equal(testUser, h.StartedBy)
	}
}

func (s *HostConnectorSuite) TestStatusFiltering() {
	hosts, err := s.ctx.FindHostsById("", "", "", 100)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		statusFound := false
		for _, status := range evergreen.UpHostStatus {
			if h.Status == status {
				statusFound = true
			}
		}
		s.True(statusFound)
	}
}

func (s *HostConnectorSuite) TestLimit() {
	hosts, err := s.ctx.FindHostsById("", evergreen.HostTerminated, "", 2)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(2, len(hosts))
	s.Equal("host2", hosts[0].Id)
	s.Equal("host3", hosts[1].Id)

	hosts, err = s.ctx.FindHostsById("", evergreen.HostTerminated, "", 3)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(3, len(hosts))
}

func (s *HostConnectorSuite) TestSpawnHost() {
	testDistroID := util.RandomString()
	const testPublicKey = "ssh-rsa 1234567890abcdef"
	const testPublicKeyName = "testPubKey"
	const testUserID = "TestSpawnHostUser"
	const testUserAPIKey = "testApiKey"
	const testUserdata = "this is a dummy sentence"
	const testInstanceType = "testInstanceType"

	config, err := evergreen.GetConfig()
	s.NoError(err)
	config.SpawnHostsPerUser = cloud.DefaultMaxSpawnHostsPerUser
	s.NoError(config.Set())

	d := &distro.Distro{
		Id:                   testDistroID,
		SpawnAllowed:         true,
		Provider:             evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", evergreen.DefaultEC2Region))},
	}
	s.NoError(d.Insert())
	testUser := &user.DBUser{
		Id:     testUserID,
		APIKey: testUserAPIKey,
	}
	testUser.PubKeys = append(testUser.PubKeys, user.PubKey{
		Name: testPublicKeyName,
		Key:  testPublicKey,
	})
	s.NoError(testUser.Insert())

	options := &restmodel.HostRequestOptions{
		DistroID:     testDistroID,
		TaskID:       "",
		KeyName:      testPublicKeyName,
		UserData:     testUserdata,
		InstanceTags: nil,
	}

	intentHost, err := (&DBHostConnector{}).NewIntentHost(context.Background(), options, testUser, config)
	s.NoError(err)
	s.Require().NotNil(intentHost)
	foundHost, err := host.FindOne(host.ById(intentHost.Id))
	s.NotNil(foundHost)
	s.NoError(err)
	s.True(foundHost.UserHost)
	s.Equal(testUserID, foundHost.StartedBy)

	s.Require().Len(foundHost.Distro.ProviderSettingsList, 1)
	ec2Settings := &cloud.EC2ProviderSettings{}
	s.NoError(ec2Settings.FromDistroSettings(foundHost.Distro, ""))
	s.Equal(ec2Settings.UserData, options.UserData)

	// with instance type
	options.InstanceType = testInstanceType
	_, err = (&DBHostConnector{}).NewIntentHost(context.Background(), options, testUser, config)
	s.Require().Error(err)
	fmt.Println(err.Error())
	s.Contains(err.Error(), "not been allowed by admins")
	config.Providers.AWS.AllowedInstanceTypes = []string{testInstanceType}
	s.NoError(config.Set())

	// found instance type in config
	_, err = (&DBHostConnector{}).NewIntentHost(context.Background(), options, testUser, config)
	s.Require().Error(err)
	fmt.Println(err.Error())
	s.Contains(err.Error(), "Unable to find region")
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

func (s *HostConnectorSuite) TestCheckHostSecret() {
	r := &http.Request{
		Header: http.Header{
			evergreen.HostHeader: []string{"host1"},
		},
	}

	code, err := s.ctx.CheckHostSecret(r)
	s.Error(err)
	s.Equal(http.StatusBadRequest, code)

	r.Header.Set(evergreen.HostSecretHeader, "abcdef")
	code, err = s.ctx.CheckHostSecret(r)
	s.NoError(err)
	s.Equal(http.StatusOK, code)
}
