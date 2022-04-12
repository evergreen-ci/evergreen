package data

import (
	"context"
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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	testutil.Setup()
}

type HostConnectorSuite struct {
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

func (*HostConnectorSuite) users() []user.DBUser {
	return []user.DBUser{
		{
			Id: testUser,
		},
		{
			Id: "user2",
		},
		{
			Id: "user3",
		},
		{
			Id: "user4",
		},
		{
			Id:          "root",
			SystemRoles: []string{"root"},
		},
	}
}

func TestHostConnectorSuite(t *testing.T) {
	// kim: TODO: remove
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	// evergreen.SetEnvironment(env)
	s := new(HostConnectorSuite)

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(user.Collection, host.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
		// kim: TODO: remove
		// cmd := map[string]string{
		//     "create": evergreen.ScopeCollection,
		// }
		// _ = evergreen.GetEnvironment().DB().RunCommand(nil, cmd)
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

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
		rm := env.RoleManager()
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

	intentHost, err := NewIntentHost(ctx, options, testUser, config)
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
	_, err = NewIntentHost(ctx, options, testUser, config)
	s.Require().Error(err)
	s.Contains(err.Error(), "not been allowed by admins")
	config.Providers.AWS.AllowedInstanceTypes = []string{testInstanceType}
	s.NoError(config.Set())
}

func (s *HostConnectorSuite) TestFindHostByIdWithOwner() {
	u, err := user.FindOneById(testUser)
	s.NoError(err)

	h, err := FindHostByIdWithOwner("host1", u)
	s.NoError(err)
	s.NotNil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdFailsWithWrongUser() {
	u, err := user.FindOneById(testUser)
	s.NoError(err)
	s.NotNil(u)

	h, err := FindHostByIdWithOwner("host2", u)
	s.Error(err)
	s.Nil(h)
}

func (s *HostConnectorSuite) TestFindHostByIdWithSuperUser() {
	u, err := user.FindOneById("root")
	s.NoError(err)

	h, err := FindHostByIdWithOwner("host2", u)
	s.NoError(err)
	s.NotNil(h)
}

func (s *HostConnectorSuite) TestGenerateHostProvisioningScriptSucceeds() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.withNewEnvironment(ctx, func() {
		script, err := GenerateHostProvisioningScript(ctx, "host1")
		s.Require().NoError(err)
		s.NotZero(script)
	})

}

func (s *HostConnectorSuite) TestGenerateHostProvisioningScriptFailsWithInvalidHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.withNewEnvironment(ctx, func() {
		script, err := GenerateHostProvisioningScript(ctx, "foo")
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
