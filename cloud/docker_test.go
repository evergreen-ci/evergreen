package cloud

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

type DockerSuite struct {
	client     DockerClient
	manager    *dockerManager
	hostOpts   host.CreateOptions
	parentHost host.Host
	env        evergreen.Environment
	suite.Suite
}

func TestDockerSuite(t *testing.T) {
	suite.Run(t, new(DockerSuite))
}

func (s *DockerSuite) SetupSuite() {
	s.env = evergreen.GetEnvironment()
	s.NoError(db.Clear(host.Collection))
}

func (s *DockerSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.client = &dockerClientMock{
		hasOpenPorts: true,
	}
	s.manager = &dockerManager{
		client: s.client,
		env:    s.env,
	}
	settingsList := []*birch.Document{birch.NewDocument(birch.EC.String("pool_id", "pool_id"))}
	s.parentHost = host.Host{
		Id:            "parent",
		Host:          "host",
		HasContainers: true,
	}
	s.hostOpts = host.CreateOptions{
		Distro: distro.Distro{
			Id:                   "d",
			Provider:             evergreen.ProviderNameDocker,
			ProviderSettingsList: settingsList,
			User:                 "root",
		},
		ParentID: "parent",
		DockerOptions: host.DockerOptions{
			Image: "http://0.0.0.0:8000/docker_image.tgz",
		},
	}
	s.NoError(s.parentHost.Insert(ctx))
}

func (s *DockerSuite) TearDownTest() {
	s.NoError(db.Clear(host.Collection))
}

func (s *DockerSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failInit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	settings := s.env.Settings()

	s.NoError(s.manager.Configure(ctx, settings))

	mock.failInit = true
	s.Error(s.manager.Configure(ctx, settings))
}

func (s *DockerSuite) TestTerminateInstanceAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostA := host.NewIntent(s.hostOpts)
	s.NoError(hostA.Insert(ctx))
	hostA, err := s.manager.SpawnHost(ctx, hostA)
	s.NoError(err)
	s.Require().NotNil(hostA)
	_, err = hostA.Upsert(ctx)
	s.NoError(err)

	hostB := host.NewIntent(s.hostOpts)
	s.NoError(hostB.Insert(ctx))
	hostB, err = s.manager.SpawnHost(ctx, hostB)
	s.NoError(err)
	s.Require().NotNil(hostB)
	_, err = hostB.Upsert(ctx)
	s.NoError(err)

	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failRemove)

	s.NoError(s.manager.TerminateInstance(ctx, hostA, evergreen.User, ""))

	mock.failRemove = true
	s.Error(s.manager.TerminateInstance(ctx, hostB, evergreen.User, ""))
}

func (s *DockerSuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	myHost := host.NewIntent(s.hostOpts)
	s.NoError(myHost.Insert(ctx))
	myHost, err := s.manager.SpawnHost(ctx, myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert(ctx)
	s.NoError(err)

	dbHost, err := host.FindOne(ctx, host.ById(myHost.Id))
	s.NotEqual(evergreen.HostTerminated, dbHost.Status)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.NoError(err)

	dbHost, err = host.FindOne(ctx, host.ById(myHost.Id))
	s.Equal(evergreen.HostTerminated, dbHost.Status)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.Error(err)
}

func (s *DockerSuite) TestSpawnInvalidSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ec2HostOps := s.hostOpts
	ec2HostOps.Distro.Provider = evergreen.ProviderNameEc2Fleet
	h := host.NewIntent(ec2HostOps)
	h, err := s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	emptyHostOpts := host.CreateOptions{Distro: s.hostOpts.Distro}
	h = host.NewIntent(emptyHostOpts)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Contains(err.Error(), "image must not be empty")
	s.Nil(h)
}

func (s *DockerSuite) TestSpawnDuplicateHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := host.NewIntent(s.hostOpts)
	s.NoError(hostOne.Insert(ctx))
	hostOne, err := s.manager.SpawnHost(ctx, hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := host.NewIntent(s.hostOpts)
	s.NoError(hostTwo.Insert(ctx))
	hostTwo, err = s.manager.SpawnHost(ctx, hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *DockerSuite) TestSpawnCreateAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := host.NewIntent(s.hostOpts)
	s.NoError(h.Insert(ctx))
	h, err := s.manager.SpawnHost(ctx, h)
	s.NoError(err)
	s.NotNil(h)

	mock.failCreate = true
	h = host.NewIntent(s.hostOpts)
	s.NoError(h.Insert(ctx))
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *DockerSuite) TestSpawnStartRemoveAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	intent := host.NewIntent(s.hostOpts)
	s.NoError(intent.Insert(ctx))
	h, err := s.manager.SpawnHost(ctx, intent)
	s.NoError(err)
	s.NotNil(h)

	mock.failStart = true
	h, err = s.manager.SpawnHost(ctx, intent)
	s.Error(err)
	s.Nil(h)

	mock.failRemove = true
	h, err = s.manager.SpawnHost(ctx, intent)
	s.Error(err)
	s.Nil(h)
}

func (s *DockerSuite) TestUtilToEvgStatus() {
	s.Equal(StatusRunning, toEvgStatus(&container.State{Running: true}))
	s.Equal(StatusStopped, toEvgStatus(&container.State{Paused: true}))
	s.Equal(StatusInitializing, toEvgStatus(&container.State{Restarting: true}))
	s.Equal(StatusTerminated, toEvgStatus(&container.State{OOMKilled: true}))
	s.Equal(StatusTerminated, toEvgStatus(&container.State{Dead: true}))
	s.Equal(StatusUnknown, toEvgStatus(&container.State{}))
}

func (s *DockerSuite) TestSpawnDoesNotPanic() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	intent := host.NewIntent(s.hostOpts)

	s.NotPanics(func() {
		_, err := s.manager.SpawnHost(ctx, intent)
		s.Error(err)
	})
}

func (s *DockerSuite) TestGetContainers() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failList)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	containers, err := s.manager.GetContainers(ctx, parent)
	s.NoError(err)
	s.Len(containers, 1)
	s.Equal("container-1", containers[0])
}

func (s *DockerSuite) TestRemoveOldestImage() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failRemove)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.RemoveOldestImage(ctx, parent)
	s.NoError(err)
}

func (s *DockerSuite) TestGetContainerImage() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failDownload)
	s.False(mock.failBuild)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.GetContainerImage(ctx, parent, host.DockerOptions{Image: "image-url", Method: distro.DockerImageBuildTypeImport})
	s.NoError(err)
}

func (s *DockerSuite) TestGetContainerImageNoBuild() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failDownload)
	s.False(mock.failBuild)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.GetContainerImage(ctx, parent, host.DockerOptions{Image: "image-url", Method: distro.DockerImageBuildTypeImport, SkipImageBuild: true})
	s.NoError(err)
}

func (s *DockerSuite) TestGetContainerImageFailedDownload() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failBuild)

	mock.failDownload = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.GetContainerImage(ctx, parent, host.DockerOptions{Image: "image-url", Method: distro.DockerImageBuildTypeImport})
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to download image")
}

func (s *DockerSuite) TestGetContainerImageFailedBuild() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failDownload)

	mock.failBuild = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId(ctx, "parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.GetContainerImage(ctx, parent, host.DockerOptions{Image: "image-url", Method: distro.DockerImageBuildTypeImport})
	s.Require().Error(err)
	s.Contains(err.Error(), "failed to build image with agent")
}
