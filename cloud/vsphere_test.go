package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
	"github.com/vmware/govmomi/vim25/types"
)

type VSphereSuite struct {
	client   vsphereClient
	manager  *vsphereManager
	hostOpts host.CreateOptions
	suite.Suite
}

func TestVSphereSuite(t *testing.T) {
	suite.Run(t, new(VSphereSuite))
}

func (s *VSphereSuite) SetupTest() {
	s.client = &vsphereClientMock{
		isActive: true,
	}
	s.manager = &vsphereManager{
		client: s.client,
	}
	s.hostOpts = host.CreateOptions{
		Distro: distro.Distro{
			Id:                   "host",
			Provider:             evergreen.ProviderNameVsphere,
			ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("template", "macos-1012"))},
		},
	}
}

func (s *VSphereSuite) TestValidateSettings() {
	// all settings are provided
	settingsOk := &vsphereSettings{
		Template:     "macos-1012",
		Datastore:    "1TB_SSD",
		ResourcePool: "XSERVE_Cluster",
		NumCPUs:      2,
		MemoryMB:     2048,
	}
	s.NoError(settingsOk.Validate())

	// only required settings are provided
	settingsMinimal := &vsphereSettings{
		Template: "macos-1012",
	}
	s.NoError(settingsMinimal.Validate())

	// error when invalid NumCPUs setting
	settingsInvalidNumCPUs := &vsphereSettings{
		Template: "macos-1012",
		NumCPUs:  -1,
	}
	s.Error(settingsInvalidNumCPUs.Validate())

	// error when invalid MemoryMB setting
	settingsInvalidMemoryMB := &vsphereSettings{
		Template: "macos-1012",
		MemoryMB: -1,
	}
	s.Error(settingsInvalidMemoryMB.Validate())
}

func (s *VSphereSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*vsphereClientMock)
	s.True(ok)
	s.False(mock.failInit)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(ctx, settings))

	mock.failInit = true
	s.Error(s.manager.Configure(ctx, settings))
}

func (s *VSphereSuite) TestTerminateInstanceAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostA := host.NewIntent(s.hostOpts)
	hostA, err := s.manager.SpawnHost(ctx, hostA)
	s.NotNil(hostA)
	s.NoError(err)
	err = hostA.Insert()
	s.NoError(err)

	hostB := host.NewIntent(s.hostOpts)
	hostB, err = s.manager.SpawnHost(ctx, hostB)
	s.NotNil(hostB)
	s.NoError(err)
	err = hostB.Insert()
	s.NoError(err)

	mock, ok := s.client.(*vsphereClientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(ctx, hostA, evergreen.User, ""))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(ctx, hostB, evergreen.User, ""))
}

func (s *VSphereSuite) TestTerminateInstanceDB() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spawn the instance - check the host is not terminated in DB.
	myHost := host.NewIntent(s.hostOpts)
	err := myHost.Insert()
	s.NoError(err)
	myHost, err = s.manager.SpawnHost(ctx, myHost)
	s.NotNil(myHost)
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.Error(err)
}

func (s *VSphereSuite) TestGetDNSNameAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock, ok := s.client.(*vsphereClientMock)
	s.True(ok)
	s.False(mock.failIP)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(ctx, host)
	s.NoError(err)

	mock.failIP = true
	dns, err := s.manager.GetDNSName(ctx, host)
	s.Error(err)
	s.Empty(dns)
}

func (s *VSphereSuite) TestSpawnInvalidSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.hostOpts.Distro = distro.Distro{Provider: evergreen.ProviderNameEc2Fleet}
	h := host.NewIntent(s.hostOpts)
	s.NotNil(h)
	h, err := s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	s.hostOpts.Distro = distro.Distro{Provider: evergreen.ProviderNameVsphere}
	h = host.NewIntent(s.hostOpts)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	s.hostOpts.Distro = distro.Distro{
		Provider:             evergreen.ProviderNameVsphere,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("template", ""))},
	}
	h = host.NewIntent(s.hostOpts)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *VSphereSuite) TestSpawnDuplicateHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := host.NewIntent(s.hostOpts)
	hostOne, err := s.manager.SpawnHost(ctx, hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := host.NewIntent(s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(ctx, hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *VSphereSuite) TestSpawnAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock, ok := s.client.(*vsphereClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	h := host.NewIntent(s.hostOpts)
	h, err := s.manager.SpawnHost(ctx, h)
	s.NoError(err)
	s.NotNil(h)

	mock.failCreate = true
	h = host.NewIntent(s.hostOpts)
	_, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
}

func (s *VSphereSuite) TestUtilToEvgStatus() {
	poweredOn := vsphereToEvgStatus(types.VirtualMachinePowerStatePoweredOn)
	s.Equal(StatusRunning, poweredOn)

	poweredOff := vsphereToEvgStatus(types.VirtualMachinePowerStatePoweredOff)
	s.Equal(StatusStopped, poweredOff)

	suspended := vsphereToEvgStatus(types.VirtualMachinePowerStateSuspended)
	s.Equal(StatusStopped, suspended)

	unknown := vsphereToEvgStatus(types.VirtualMachinePowerState("???"))
	s.Equal(StatusUnknown, unknown)
}
