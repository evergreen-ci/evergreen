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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

func init() {
	testutil.Setup()
}

type HostConnectorSuite struct {
	conn  Connector
	setup func(*HostConnectorSuite)
	suite.Suite
}

const testUser = "user1"

func (*HostConnectorSuite) hosts() []host.Host {
	return []host.Host{
		{
			Id:        "host1",
			StartedBy: testUser,
			Distro: distro.Distro{
				Id:      "distro1",
				Aliases: []string{"alias125"},
				Arch:    evergreen.ArchLinuxAmd64,
			},
			Status:         evergreen.HostRunning,
			ExpirationTime: time.Now().Add(time.Hour),
			Secret:         "abcdef",
			IP:             "ip1",
		}, {
			Id:        "host2",
			StartedBy: "user2",
			Distro: distro.Distro{
				Id:      "distro2",
				Aliases: []string{"alias125"},
			},
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip2",
		}, {
			Id:             "host3",
			StartedBy:      "user3",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip3",
		}, {
			Id:             "host4",
			StartedBy:      "user4",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip4",
		}, {
			Id:        "host5",
			StartedBy: evergreen.User,
			Status:    evergreen.HostRunning,
			Distro: distro.Distro{
				Id:      "distro5",
				Aliases: []string{"alias125"},
			},
			IP: "ip5",
		},
	}
}

func TestHostConnectorSuite(t *testing.T) {
	s := new(HostConnectorSuite)
	s.conn = &DBConnector{}

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(user.Collection, host.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
		cmd := map[string]string{
			"create": evergreen.ScopeCollection,
		}
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)

		hosts := s.hosts()
		for _, h := range hosts {
			s.Require().NoError(h.Insert())
		}

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
		s.conn = &MockConnector{
			MockHostConnector: MockHostConnector{
				CachedHosts: s.hosts(),
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

func (s *HostConnectorSuite) TestFindById() {
	h1, ok := s.conn.FindHostById("host1")
	s.NoError(ok)
	s.NotNil(h1)
	s.Equal("host1", h1.Id)

	h2, ok := s.conn.FindHostById("host2")
	s.NoError(ok)
	s.NotNil(h2)
	s.Equal("host2", h2.Id)
}

func (s *HostConnectorSuite) TestFindByIdFail() {
	h, ok := s.conn.FindHostById("nonexistent")
	s.NoError(ok)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindByIP() {
	h1, ok := s.conn.FindHostByIpAddress("ip1")
	s.NoError(ok)
	s.NotNil(h1)
	s.Equal("host1", h1.Id)
	s.Equal("ip1", h1.IP)

	h2, ok := s.conn.FindHostByIpAddress("ip2")
	s.NoError(ok)
	s.NotNil(h2)
	s.Equal("host2", h2.Id)
	s.Equal("ip2", h2.IP)
}

func (s *HostConnectorSuite) TestFindByIPFail() {
	h, ok := s.conn.FindHostByIpAddress("nonexistent")
	s.NoError(ok)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindHostsByDistro() {
	hosts, err := s.conn.FindHostsByDistro("distro5")
	s.Require().NoError(err)
	s.Require().Len(hosts, 1)
	s.Equal("host5", hosts[0].Id)

	hosts, err = s.conn.FindHostsByDistro("alias125")
	s.Require().NoError(err)
	s.Require().Len(hosts, 2)
	var host1Found, host5Found bool
	for _, h := range hosts {
		if h.Id == "host1" {
			host1Found = true
		}
		if h.Id == "host5" {
			host5Found = true
		}
	}
	s.True(host1Found)
	s.True(host5Found)
}

func (s *HostConnectorSuite) TestFindByUser() {
	hosts, err := s.conn.FindHostsById("", "", testUser, 100)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		s.Equal(testUser, h.StartedBy)
	}
}

func (s *HostConnectorSuite) TestStatusFiltering() {
	hosts, err := s.conn.FindHostsById("", "", "", 100)
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
	hosts, err := s.conn.FindHostsById("", evergreen.HostTerminated, "", 2)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(2, len(hosts))
	s.Equal("host2", hosts[0].Id)
	s.Equal("host3", hosts[1].Id)

	hosts, err = s.conn.FindHostsById("", evergreen.HostTerminated, "", 3)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(3, len(hosts))
}

func (s *HostConnectorSuite) TestSpawnHost() {
	testDistroID := utility.RandomString()
	const testPublicKey = "ssh-rsa 1234567890abcdef"
	const testPublicKeyName = "testPubKey"
	const testUserID = "TestSpawnHostUser"
	const testUserAPIKey = "testApiKey"
	const testUserdata = "#!/bin/bash\necho this is a dummy sentence"
	const testInstanceType = "testInstanceType"

	config, err := evergreen.GetConfig()
	s.NoError(err)
	config.Spawnhost.SpawnHostsPerUser = evergreen.DefaultMaxSpawnHostsPerUser

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
	ctx := gimlet.AttachUser(context.Background(), testUser)
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

	intentHost, err := (&DBHostConnector{}).NewIntentHost(ctx, options, testUser, config)
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
	_, err = (&DBHostConnector{}).NewIntentHost(ctx, options, testUser, config)
	s.Require().Error(err)
	s.Contains(err.Error(), "not been allowed by admins")
	config.Providers.AWS.AllowedInstanceTypes = []string{testInstanceType}
	s.NoError(config.Set())
}

func (s *HostConnectorSuite) TestSetHostStatus() {
	h, err := s.conn.FindHostById("host1")
	s.NoError(err)
	s.NoError(s.conn.SetHostStatus(h, evergreen.HostTerminated, evergreen.User))

	for i := 1; i < 5; i++ {
		h, err := s.conn.FindHostById(fmt.Sprintf("host%d", i))
		s.NoError(err)
		s.Equal(evergreen.HostTerminated, h.Status)
	}
}

func (s *HostConnectorSuite) TestExtendHostExpiration() {
	h, err := s.conn.FindHostById("host1")
	s.NoError(err)
	expectedTime := h.ExpirationTime.Add(5 * time.Hour)
	s.NoError(s.conn.SetHostExpirationTime(h, expectedTime))

	hCheck, err := s.conn.FindHostById("host1")
	s.Equal(expectedTime, hCheck.ExpirationTime)
	s.NoError(err)
}

func (s *HostConnectorSuite) TestFindHostByIdWithOwner() {
	u, err := s.conn.FindUserById(testUser)
	s.NoError(err)

	h, err := s.conn.FindHostByIdWithOwner("host1", u)
	s.NoError(err)
	s.NotNil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdFailsWithWrongUser() {
	u, err := s.conn.FindUserById(testUser)
	s.NoError(err)
	s.NotNil(u)

	h, err := s.conn.FindHostByIdWithOwner("host2", u)
	s.Error(err)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdWithSuperUser() {
	u, err := s.conn.FindUserById("root")
	s.NoError(err)

	h, err := s.conn.FindHostByIdWithOwner("host2", u)
	s.NoError(err)
	s.NotNil(h)
}

func (s *HostConnectorSuite) TestCheckHostSecret() {
	r := &http.Request{
		Header: http.Header{
			evergreen.HostHeader: []string{"host1"},
		},
	}

	code, err := s.conn.CheckHostSecret("", r)
	s.Error(err)
	s.Equal(http.StatusBadRequest, code)

	r.Header.Set(evergreen.HostSecretHeader, "abcdef")
	code, err = s.conn.CheckHostSecret("host1", r)
	s.NoError(err)
	s.Equal(http.StatusOK, code)

	code, err = s.conn.CheckHostSecret("", r)
	s.NoError(err)
	s.Equal(http.StatusOK, code)
}

func (s *HostConnectorSuite) TestGenerateHostProvisioningScriptSucceeds() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.withNewEnvironment(ctx, func() {
		script, err := s.conn.GenerateHostProvisioningScript(ctx, "host1")
		s.Require().NoError(err)
		s.NotZero(script)
	})

}

func (s *HostConnectorSuite) TestGenerateHostProvisioningScriptFailsWithInvalidHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.withNewEnvironment(ctx, func() {
		script, err := s.conn.GenerateHostProvisioningScript(ctx, "foo")
		s.Error(err)
		s.Zero(script)
	})
}

func (s *HostConnectorSuite) withNewEnvironment(ctx context.Context, testCase func()) {
	// We have to set the environment temporarily because the suites in this
	// package drop the database, but some host tests require that host
	// credentials collection to be bootstrapped with the CA.
	oldEnv := evergreen.GetEnvironment()
	defer func() {
		evergreen.SetEnvironment(oldEnv)
	}()
	env := testutil.NewEnvironment(ctx, s.T())
	evergreen.SetEnvironment(env)

	testCase()
}
