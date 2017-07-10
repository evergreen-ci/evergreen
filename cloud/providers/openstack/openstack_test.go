package openstack

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"

	"github.com/stretchr/testify/suite"
)

type OpenStackSuite struct {
	client       client
	keyname      string
	manager      *Manager
	distro       *distro.Distro
	hostOpts     cloud.HostOptions
	suite.Suite
}

func TestOpenStackSuite(t *testing.T) {
	suite.Run(t, new(OpenStackSuite))
}

func (s *OpenStackSuite) SetupSuite() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func (s *OpenStackSuite) SetupTest() {
	s.client  = &clientMock{isServerActive: true}
	s.keyname = "key"

	s.manager = &Manager{
		client: s.client,
	}

	s.distro = &distro.Distro{
		Id:       "host",
		Provider: "openstack",
		ProviderSettings: &map[string]interface{}{
			"image_name":     "image",
			"flavor_name":    "flavor",
			"key_name":       "key",
			"security_group": "group",
		},
	}
	s.hostOpts = cloud.HostOptions{}
}

func (s *OpenStackSuite) TestValidateSettings() {
	settingsOk := &ProviderSettings{
		ImageName:  "image",
		FlavorName: "flavor",
		KeyName:    "key",
	}
	s.NoError(settingsOk.Validate())

	settingsNoImage := &ProviderSettings{
		FlavorName: "flavor",
		KeyName:    "key",
	}
	s.Error(settingsNoImage.Validate())

	settingsNoFlavor := &ProviderSettings{
		ImageName:  "image",
		KeyName:    "key",
	}
	s.Error(settingsNoFlavor.Validate())

	settingsNoKey := &ProviderSettings{
		ImageName:  "image",
		FlavorName: "flavor",
	}
	s.Error(settingsNoKey.Validate())
}

func (s *OpenStackSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failInit)

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(settings))

	mock.failInit = true
	s.Error(s.manager.Configure(settings))
}

func (s *OpenStackSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)

	host := &host.Host{}

	mock.failGet = true
	_, err := s.manager.GetInstanceStatus(host)
	s.Error(err)

	active, err := s.manager.IsUp(host)
	s.Error(err)
	s.False(active)
}

func (s *OpenStackSuite) TestIsUpStatuses() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.True(mock.isServerActive)

	host := &host.Host{}

	status, err := s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.Equal(cloud.StatusRunning, status)

	active, err := s.manager.IsUp(host)
	s.NoError(err)
	s.True(active)

	mock.isServerActive = false
	status, err = s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.NotEqual(cloud.StatusRunning, status)

	active, err = s.manager.IsUp(host)
	s.NoError(err)
	s.False(active)
}

func (s *OpenStackSuite) TestTerminateInstanceAPICall() {
	hostA, err := s.manager.SpawnInstance(s.distro, s.hostOpts)
	s.NotNil(hostA)
	s.NoError(err)

	hostB, err := s.manager.SpawnInstance(s.distro, s.hostOpts)
	s.NotNil(hostB)
	s.NoError(err)

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(hostA))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(hostB))
}

func (s *OpenStackSuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	myHost, err := s.manager.SpawnInstance(s.distro, s.hostOpts)
	s.NotNil(myHost)
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(myHost)
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(myHost)
	s.Error(err)
}

func (s *OpenStackSuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failGet)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(host)
	s.NoError(err)

	mock.failGet = true
	dns, err := s.manager.GetDNSName(host)
	s.Error(err)
	s.Empty(dns)
}

func (s *OpenStackSuite) TestGetSSHOptions() {
	opt := "Option"
	host := &host.Host{
		Distro: distro.Distro {
			SSHOptions: []string{opt},
		},
	}

	opts, err := s.manager.GetSSHOptions(host, "")
	s.Error(err)
	s.Empty(opts)

	ok, err := s.manager.IsSSHReachable(host, "")
	s.Error(err)
	s.False(ok)

	opts, err = s.manager.GetSSHOptions(host, s.keyname)
	s.NoError(err)
	s.Equal([]string{"-i", s.keyname, "-o", opt}, opts)
}

func (s *OpenStackSuite) TestSpawnInvalidSettings() {
	dProviderName := &distro.Distro{Provider: "ec2"}
	host, err := s.manager.SpawnInstance(dProviderName, s.hostOpts)
	s.Error(err)
	s.Nil(host)

	dSettingsNone := &distro.Distro{Provider: "openstack"}
	host, err = s.manager.SpawnInstance(dSettingsNone, s.hostOpts)
	s.Error(err)
	s.Nil(host)

	dSettingsInvalid := &distro.Distro{
		Provider: "openstack",
		ProviderSettings: &map[string]interface{}{"image_name": ""},
	}
	host, err = s.manager.SpawnInstance(dSettingsInvalid, s.hostOpts)
	s.Error(err)
	s.Nil(host)
}

func (s *OpenStackSuite) TestSpawnDuplicateHostID() {
	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne, err := s.manager.SpawnInstance(s.distro, s.hostOpts)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo, err := s.manager.SpawnInstance(s.distro, s.hostOpts)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *OpenStackSuite) TestSpawnAPICall() {
	dist := &distro.Distro{
		Id:       "id",
		Provider: "openstack",
		ProviderSettings: &map[string]interface{}{
			"image_name":     "image",
			"flavor_name":    "flavor",
			"key_name":       "key",
			"security_group": "group",
		},
	}

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failCreate)

	host, err := s.manager.SpawnInstance(dist, s.hostOpts)
	s.NoError(err)
	s.NotNil(host)

	mock.failCreate = true
	host, err = s.manager.SpawnInstance(dist, s.hostOpts)
	s.Error(err)
	s.Nil(host)
}

func (s *OpenStackSuite) TestUtilToEvgStatus() {
	s.Equal(cloud.StatusRunning, osStatusToEvgStatus("ACTIVE"))
	s.Equal(cloud.StatusRunning, osStatusToEvgStatus("IN_PROGRESS"))
	s.Equal(cloud.StatusStopped, osStatusToEvgStatus("SHUTOFF"))
	s.Equal(cloud.StatusInitializing, osStatusToEvgStatus("BUILD"))
	s.Equal(cloud.StatusUnknown, osStatusToEvgStatus("???"))
}
