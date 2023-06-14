package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	testutil.Setup()
}

type HostConnectorSuite struct {
	env   evergreen.Environment
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := new(HostConnectorSuite)
	s.env = testutil.NewEnvironment(ctx, t)

	s.setup = func(s *HostConnectorSuite) {
		s.NoError(db.ClearCollections(user.Collection, host.Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
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
		rm := s.env.RoleManager()
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testDistroID := utility.RandomString()
	const testPublicKey = "ssh-rsa 1234567890abcdef"
	const testPublicKeyName = "testPubKey"
	const testUserID = "TestSpawnHostUser"
	const testUserAPIKey = "testApiKey"
	const testUserdata = "#!/bin/bash\necho this is a dummy sentence"
	const testInstanceType = "testInstanceType"

	env := &mock.Environment{}
	s.NoError(env.Configure(ctx))
	env.EvergreenSettings.Spawnhost.SpawnHostsPerUser = evergreen.DefaultMaxSpawnHostsPerUser
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	s.NoError(err)

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
	ctx = gimlet.AttachUser(ctx, testUser)
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

	intentHost, err := NewIntentHost(ctx, options, testUser, env)
	s.NoError(err)
	s.Require().NotNil(intentHost)
	foundHost, err := host.FindOneByIdOrTag(intentHost.Id)
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
	_, err = NewIntentHost(ctx, options, testUser, env)
	s.Require().Error(err)
	s.Contains(err.Error(), "not been allowed by admins")
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
	script, err := GenerateHostProvisioningScript(ctx, s.env, "host1")
	s.Require().NoError(err)
	s.NotZero(script)

}

func (s *HostConnectorSuite) TestGenerateHostProvisioningScriptFailsWithInvalidHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	script, err := GenerateHostProvisioningScript(ctx, s.env, "foo")
	s.Error(err)
	s.Zero(script)
}
