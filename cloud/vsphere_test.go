// +build go1.7

package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"github.com/vmware/govmomi/vim25/types"
)

type VSphereSuite struct {
	client   vsphereClient
	manager  *vsphereManager
	distro   *distro.Distro
	hostOpts HostOptions
	suite.Suite
}

func TestVSphereSuite(t *testing.T) {
	suite.Run(t, new(VSphereSuite))
}

func (s *VSphereSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *VSphereSuite) SetupTest() {
	s.client = &vsphereClientMock{
		isActive: true,
	}
	s.manager = &vsphereManager{
		client: s.client,
	}
	s.distro = &distro.Distro{
		Id:       "host",
		Provider: "vsphere",
		ProviderSettings: &map[string]interface{}{
			"template": "macos-1012",
		},
	}
	s.hostOpts = HostOptions{}
}

func (s *VSphereSuite) TestValidateSettings() {
	// all settings are provided
	settingsOk := &ProviderSettings{
		Template:     "macos-1012",
		Datastore:    "1TB_SSD",
		ResourcePool: "XSERVE_Cluster",
		NumCPUs:      2,
		MemoryMB:     2048,
	}
	s.NoError(settingsOk.Validate())

	// only required settings are provided
	settingsMinimal := &ProviderSettings{
		Template: "macos-1012",
	}
	s.NoError(settingsMinimal.Validate())

	// error when invalid NumCPUs setting
	settingsInvalidNumCPUs := &ProviderSettings{
		Template: "macos-1012",
		NumCPUs:  -1,
	}
	s.Error(settingsInvalidNumCPUs.Validate())

	// error when invalid MemoryMB setting
	settingsInvalidMemoryMB := &ProviderSettings{
		Template: "macos-1012",
		MemoryMB: -1,
	}
	s.Error(settingsInvalidMemoryMB.Validate())
}

func (s *VSphereSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failInit)

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(settings))

	mock.failInit = true
	s.Error(s.manager.Configure(settings))
}

func (s *VSphereSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)

	host := &host.Host{}

	mock.failPowerState = true
	_, err := s.manager.GetInstanceStatus(host)
	s.Error(err)

	active, err := s.manager.IsUp(host)
	s.Error(err)
	s.False(active)
}

func (s *VSphereSuite) TestIsUpStatuses() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.True(mock.isActive)

	host := &host.Host{}

	status, err := s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	active, err := s.manager.IsUp(host)
	s.NoError(err)
	s.True(active)

	mock.isActive = false
	status, err = s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.NotEqual(StatusRunning, status)

	active, err = s.manager.IsUp(host)
	s.NoError(err)
	s.False(active)
}

func (s *VSphereSuite) TestTerminateInstanceAPICall() {
	hostA := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(hostA)
	s.NotNil(hostA)
	s.NoError(err)
	err = hostA.Insert()
	s.NoError(err)

	hostB := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(hostB)
	s.NotNil(hostB)
	s.NoError(err)
	err = hostB.Insert()
	s.NoError(err)

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(hostA))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(hostB))
}

func (s *VSphereSuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	myHost := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	err := myHost.Insert()
	s.NoError(err)
	myHost, err = s.manager.SpawnHost(myHost)
	s.NotNil(myHost)
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(myHost)
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(myHost)
	s.Error(err)
}

func (s *VSphereSuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failIP)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(host)
	s.NoError(err)

	mock.failIP = true
	dns, err := s.manager.GetDNSName(host)
	s.Error(err)
	s.Empty(dns)
}

func (s *VSphereSuite) TestGetSSHOptions() {
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

	ok, err := s.manager.IsSSHReachable(host, "")
	s.Error(err)
	s.False(ok)

	opts, err = s.manager.GetSSHOptions(host, keyname)
	s.NoError(err)
	s.Equal([]string{"-i", keyname, "-o", opt}, opts)
}

func (s *VSphereSuite) TestSpawnInvalidSettings() {
	dProviderName := &distro.Distro{Provider: "ec2"}
	host := NewIntent(*dProviderName, s.manager.GetInstanceName(dProviderName), dProviderName.Provider, s.hostOpts)
	s.NotNil(host)
	host, err := s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)

	dSettingsNone := &distro.Distro{Provider: "vsphere"}
	host = NewIntent(*dSettingsNone, s.manager.GetInstanceName(dSettingsNone), dSettingsNone.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)

	dSettingsInvalid := &distro.Distro{
		Provider:         "vsphere",
		ProviderSettings: &map[string]interface{}{"template": ""},
	}
	host = NewIntent(*dSettingsInvalid, s.manager.GetInstanceName(dSettingsInvalid), dSettingsInvalid.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)
}

func (s *VSphereSuite) TestSpawnDuplicateHostID() {
	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostOne, err := s.manager.SpawnHost(hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *VSphereSuite) TestSpawnAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failCreate)

	host := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(host)
	s.NoError(err)
	s.NotNil(host)

	mock.failCreate = true
	host = NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	_, err = s.manager.SpawnHost(host)
	s.Error(err)
}

func (s *VSphereSuite) TestUtilToEvgStatus() {
	poweredOn := toEvgStatus(types.VirtualMachinePowerStatePoweredOn)
	s.Equal(StatusRunning, poweredOn)

	poweredOff := toEvgStatus(types.VirtualMachinePowerStatePoweredOff)
	s.Equal(StatusStopped, poweredOff)

	suspended := toEvgStatus(types.VirtualMachinePowerStateSuspended)
	s.Equal(StatusStopped, suspended)

	unknown := toEvgStatus(types.VirtualMachinePowerState("???"))
	s.Equal(StatusUnknown, unknown)
}
