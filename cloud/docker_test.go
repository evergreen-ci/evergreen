// +build go1.7

package cloud

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type DockerSuite struct {
	client     dockerClient
	manager    *dockerManager
	distro     distro.Distro
	hostOpts   HostOptions
	parentHost host.Host
	suite.Suite
}

func TestDockerSuite(t *testing.T) {
	suite.Run(t, new(DockerSuite))
}

func (s *DockerSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.NoError(db.Clear(host.Collection))
}

func (s *DockerSuite) SetupTest() {
	s.client = &dockerClientMock{
		hasOpenPorts: true,
	}
	s.manager = &dockerManager{
		client: s.client,
	}
	s.distro = distro.Distro{
		Id:       "d",
		Provider: "docker",
		ProviderSettings: &map[string]interface{}{
			"image_name": "docker_image",
			"pool_id":    "pool_id",
		},
	}
	s.parentHost = host.Host{
		Id:            "parent",
		Host:          "host",
		HasContainers: true,
	}
	s.hostOpts = HostOptions{
		ParentID: "parent",
	}
	s.NoError(s.parentHost.Insert())
	s.NoError(s.distro.Insert())
}

func (s *DockerSuite) TearDownTest() {
	s.NoError(db.ClearCollections(host.Collection, distro.Collection))
}
func (s *DockerSuite) TestValidateSettings() {
	// all required settings are provided
	settingsOk := &dockerSettings{
		ImageID: "docker_image",
	}
	s.NoError(settingsOk.Validate())

	// error when missing image id
	settingsNoImageID := &dockerSettings{}
	s.Error(settingsNoImageID.Validate())
}

func (s *DockerSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failInit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(ctx, settings))

	mock.failInit = true
	s.Error(s.manager.Configure(ctx, settings))
}

func (s *DockerSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)

	host := &host.Host{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock.failGet = true
	_, err := s.manager.GetInstanceStatus(ctx, host)
	s.Error(err)

	active, err := s.manager.IsUp(ctx, host)
	s.Error(err)
	s.False(active)
}

func (s *DockerSuite) TestIsUpStatuses() {
	host := &host.Host{ParentID: "parent"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status, err := s.manager.GetInstanceStatus(ctx, host)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	active, err := s.manager.IsUp(ctx, host)
	s.NoError(err)
	s.True(active)
}

func (s *DockerSuite) TestTerminateInstanceAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostA := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(ctx, hostA)
	s.NotNil(hostA)
	s.NoError(err)
	_, err = hostA.Upsert()
	s.NoError(err)

	hostB := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(ctx, hostB)
	s.NotNil(hostB)
	s.NoError(err)
	_, err = hostB.Upsert()
	s.NoError(err)

	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failRemove)

	s.NoError(s.manager.TerminateInstance(ctx, hostA, evergreen.User))

	mock.failRemove = true
	s.Error(s.manager.TerminateInstance(ctx, hostB, evergreen.User))
}

func (s *DockerSuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	myHost := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	myHost, err := s.manager.SpawnHost(ctx, myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert()
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User)
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User)
	s.Error(err)
}

func (s *DockerSuite) TestGetSSHOptions() {
	opt := "Option"
	keyname := "key"
	host := &host.Host{
		Distro: distro.Distro{
			SSHOptions: []string{opt},
		},
	}

	opts, err := s.manager.GetSSHOptions(host, "")
	s.Error(err)
	s.Empty(opts)

	opts, err = s.manager.GetSSHOptions(host, keyname)
	s.NoError(err)
	s.Equal([]string{"-i", keyname, "-o", opt}, opts)
}

func (s *DockerSuite) TestSpawnInvalidSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dProviderName := distro.Distro{Provider: "ec2"}
	host := NewIntent(dProviderName, dProviderName.GenerateName(), dProviderName.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)

	dSettingsNone := distro.Distro{Provider: "docker"}
	host = NewIntent(dSettingsNone, dSettingsNone.GenerateName(), dSettingsNone.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)

	dSettingsInvalid := distro.Distro{
		Provider:         "docker",
		ProviderSettings: &map[string]interface{}{"instance_type": ""},
	}
	host = NewIntent(dSettingsInvalid, dSettingsInvalid.GenerateName(), dSettingsInvalid.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)
}

func (s *DockerSuite) TestSpawnDuplicateHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostOne, err := s.manager.SpawnHost(ctx, hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
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

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(ctx, host)
	s.NoError(err)
	s.NotNil(host)

	mock.failCreate = true
	host = NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)
}

func (s *DockerSuite) TestSpawnStartRemoveAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	h, err := s.manager.SpawnHost(ctx, host)
	s.NoError(err)
	s.NotNil(h)

	mock.failStart = true
	h, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(h)

	mock.failRemove = true
	h, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(h)
}

func (s *DockerSuite) TestSpawnFailOpenPortBinding() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.True(mock.hasOpenPorts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(ctx, host)
	s.NoError(err)
	s.NotNil(host)

	mock.hasOpenPorts = false
	host = NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)
}

func (s *DockerSuite) TestSpawnGetAPICall() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(ctx, host)
	s.NoError(err)
	s.NotNil(host)

	mock.failGet = true
	host = NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(ctx, host)
	s.Error(err)
	s.Nil(host)
}

func (s *DockerSuite) TestGetDNSName() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	dns, err := s.manager.GetDNSName(ctx, host)
	s.Error(err)
	s.Empty(dns)

	host, err = s.manager.SpawnHost(ctx, host)
	s.NoError(err)
	s.NotNil(host)

	dns, err = s.manager.GetDNSName(ctx, host)
	s.NoError(err)
	s.NotEmpty(dns)
}

func (s *DockerSuite) TestMakeHostConfig() {
	container := types.Container{
		Ports: []types.Port{
			{PublicPort: 5000},
			{PublicPort: 5001},
		},
	}
	containers := []types.Container{container}

	hostNoOpenPorts := &host.Host{
		Id: "host-1",
		ContainerPoolSettings: &evergreen.ContainerPool{
			Id:            "test_pool-1",
			MaxContainers: 1,
			Port:          5000,
		},
	}

	conf, err := makeHostConfig(hostNoOpenPorts, containers)
	s.Error(err)
	s.Nil(conf)

	hostOpenPorts := &host.Host{
		Id: "host-2",
		ContainerPoolSettings: &evergreen.ContainerPool{
			Id:            "test_pool-2",
			MaxContainers: 10,
			Port:          5000,
		},
	}

	conf, err = makeHostConfig(hostOpenPorts, containers)
	s.NoError(err)
	s.NotNil(conf)
}

func (s *DockerSuite) TestUtilToEvgStatus() {
	s.Equal(StatusRunning, toEvgStatus(&types.ContainerState{Running: true}))
	s.Equal(StatusStopped, toEvgStatus(&types.ContainerState{Paused: true}))
	s.Equal(StatusInitializing, toEvgStatus(&types.ContainerState{Restarting: true}))
	s.Equal(StatusTerminated, toEvgStatus(&types.ContainerState{OOMKilled: true}))
	s.Equal(StatusTerminated, toEvgStatus(&types.ContainerState{Dead: true}))
	s.Equal(StatusUnknown, toEvgStatus(&types.ContainerState{}))
}

func (s *DockerSuite) TestUtilRetrieveOpenPortBinding() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)

	containerNoPortBindings := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    mock.generateContainerID(),
			State: &types.ContainerState{Running: true},
		},
		Config: &container.Config{
			ExposedPorts: nat.PortSet{"22/tcp": {}},
		},
		NetworkSettings: &types.NetworkSettings{},
	}
	port, err := retrieveOpenPortBinding(containerNoPortBindings)
	s.Empty(port)
	s.Error(err)

	hostPort := "5000"
	containerOpenPortBinding := &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:    mock.generateContainerID(),
			State: &types.ContainerState{Running: true},
		},
		Config: &container.Config{
			ExposedPorts: nat.PortSet{"22/tcp": {}},
		},
		NetworkSettings: &types.NetworkSettings{
			NetworkSettingsBase: types.NetworkSettingsBase{
				Ports: nat.PortMap{
					"22/tcp": []nat.PortBinding{
						{"0.0.0.0", hostPort},
					},
				},
			},
		},
	}
	port, err = retrieveOpenPortBinding(containerOpenPortBinding)
	s.Equal(hostPort, port)
	s.NoError(err)
}

func (s *DockerSuite) TestSpawnDoesNotPanic() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	delete(*s.distro.ProviderSettings, "image_name")

	host := NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)

	s.NotPanics(func() {
		_, err := s.manager.SpawnHost(ctx, host)
		s.Error(err)
	})
}

func (s *DockerSuite) TestGetContainers() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failList)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId("parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	containers, err := s.manager.GetContainers(ctx, parent)
	s.NoError(err)
	s.Equal(1, len(containers))
	s.Equal("container-1", containers[0])
}

func (s *DockerSuite) TestRemoveLeastRecentlyUsedImageID() {
	mock, ok := s.client.(*dockerClientMock)
	s.True(ok)
	s.False(mock.failList)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parent, err := host.FindOneId("parent")
	s.NoError(err)
	s.Equal("parent", parent.Id)

	err = s.manager.RemoveLeastRecentlyUsedImageID(ctx, parent)
	s.NoError(err)
}
