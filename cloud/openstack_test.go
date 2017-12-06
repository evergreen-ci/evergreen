package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type OpenStackSuite struct {
	client   openStackClient
	keyname  string
	manager  *openStackManager
	distro   *distro.Distro
	hostOpts HostOptions
	suite.Suite
}

func TestOpenStackSuite(t *testing.T) {
	suite.Run(t, new(OpenStackSuite))
}

func (s *OpenStackSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *OpenStackSuite) SetupTest() {
	s.client = &openStackClientMock{isServerActive: true}
	s.keyname = "key"

	s.manager = &openStackManager{
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
	s.hostOpts = HostOptions{}
}

func (s *OpenStackSuite) TestValidateSettings() {
	// all required settings are provided
	settingsOk := &openStackSettings{
		ImageName:     "image",
		FlavorName:    "flavor",
		KeyName:       "key",
		SecurityGroup: "sec",
	}
	s.NoError(settingsOk.Validate())

	// error when missing image name
	settingsNoImage := &openStackSettings{
		FlavorName:    "flavor",
		KeyName:       "key",
		SecurityGroup: "sec",
	}
	s.Error(settingsNoImage.Validate())

	// error when missing flavor name
	settingsNoFlavor := &openStackSettings{
		ImageName:     "image",
		KeyName:       "key",
		SecurityGroup: "sec",
	}
	s.Error(settingsNoFlavor.Validate())

	// error when missing key name
	settingsNoKey := &openStackSettings{
		ImageName:     "image",
		FlavorName:    "flavor",
		SecurityGroup: "sec",
	}
	s.Error(settingsNoKey.Validate())

	// error when missing security group
	settingsNoSecGroup := &openStackSettings{
		ImageName:  "image",
		FlavorName: "flavor",
		KeyName:    "key",
	}
	s.Error(settingsNoSecGroup.Validate())
}

func (s *OpenStackSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.False(mock.failInit)

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(settings))

	mock.failInit = true
	s.Error(s.manager.Configure(settings))
}

func (s *OpenStackSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*openStackClientMock)
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
	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.True(mock.isServerActive)

	host := &host.Host{}

	status, err := s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	active, err := s.manager.IsUp(host)
	s.NoError(err)
	s.True(active)

	mock.isServerActive = false
	status, err = s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.NotEqual(StatusRunning, status)

	active, err = s.manager.IsUp(host)
	s.NoError(err)
	s.False(active)
}

func (s *OpenStackSuite) TestTerminateInstanceAPICall() {
	hostA := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(hostA)
	s.NotNil(hostA)
	s.NoError(err)
	_, err = hostA.Upsert()
	s.NoError(err)

	hostB := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(hostB)
	s.NotNil(hostB)
	s.NoError(err)
	_, err = hostB.Upsert()
	s.NoError(err)

	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(hostA))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(hostB))
}

func (s *OpenStackSuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	myHost := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	myHost, err := s.manager.SpawnHost(myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert()
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotNil(dbHost)
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

func (s *OpenStackSuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*openStackClientMock)
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

	opts, err = s.manager.GetSSHOptions(host, s.keyname)
	s.NoError(err)
	s.Equal([]string{"-i", s.keyname, "-o", opt}, opts)
}

func (s *OpenStackSuite) TestSpawnInvalidSettings() {
	var err error
	dProviderName := &distro.Distro{Provider: "ec2"}
	host := NewIntent(*dProviderName, s.manager.GetInstanceName(dProviderName), dProviderName.Provider, s.hostOpts)
	s.NotNil(host)
	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)

	dSettingsNone := &distro.Distro{Provider: "openstack"}
	host = NewIntent(*dSettingsNone, s.manager.GetInstanceName(dSettingsNone), dSettingsNone.Provider, s.hostOpts)
	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)

	dSettingsInvalid := &distro.Distro{
		Provider:         "openstack",
		ProviderSettings: &map[string]interface{}{"image_name": ""},
	}
	host = NewIntent(*dSettingsInvalid, s.manager.GetInstanceName(dSettingsInvalid), dSettingsInvalid.Provider, s.hostOpts)
	s.NotNil(host)
	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)
}

func (s *OpenStackSuite) TestSpawnDuplicateHostID() {
	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)

	hostOne, err := s.manager.SpawnHost(hostOne)
	s.NoError(err)
	s.NotNil(hostOne)
	_, err = hostOne.Upsert()
	s.NoError(err)

	hostTwo := NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
	_, err = hostTwo.Upsert()
	s.NoError(err)
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

	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	host := NewIntent(*dist, s.manager.GetInstanceName(dist), dist.Provider, s.hostOpts)
	host, err := s.manager.SpawnHost(host)
	s.NoError(err)
	s.NotNil(host)
	_, err = host.Upsert()
	s.NoError(err)

	mock.failCreate = true
	host = NewIntent(*dist, s.manager.GetInstanceName(dist), dist.Provider, s.hostOpts)

	host, err = s.manager.SpawnHost(host)
	s.Error(err)
	s.Nil(host)
}

func (s *OpenStackSuite) TestUtilToEvgStatus() {
	s.Equal(StatusRunning, osStatusToEvgStatus("ACTIVE"))
	s.Equal(StatusRunning, osStatusToEvgStatus("IN_PROGRESS"))
	s.Equal(StatusStopped, osStatusToEvgStatus("SHUTOFF"))
	s.Equal(StatusInitializing, osStatusToEvgStatus("BUILD"))
	s.Equal(StatusUnknown, osStatusToEvgStatus("???"))
}
