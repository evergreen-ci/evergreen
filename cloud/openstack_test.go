package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

type OpenStackSuite struct {
	client   openStackClient
	keyname  string
	manager  *openStackManager
	distro   distro.Distro
	hostOpts host.CreateOptions
	suite.Suite
}

func TestOpenStackSuite(t *testing.T) {
	suite.Run(t, new(OpenStackSuite))
}

func (s *OpenStackSuite) SetupTest() {
	s.client = &openStackClientMock{isServerActive: true}
	s.keyname = "key"

	s.manager = &openStackManager{
		client: s.client,
	}

	s.distro = distro.Distro{
		Id:       "host",
		Provider: evergreen.ProviderNameOpenstack,
		ProviderSettings: &map[string]interface{}{
			"image_name":     "image",
			"flavor_name":    "flavor",
			"key_name":       "key",
			"security_group": "group",
		},
	}
	s.hostOpts = host.CreateOptions{}
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(ctx, settings))

	mock.failInit = true
	s.Error(s.manager.Configure(ctx, settings))
}

func (s *OpenStackSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := &host.Host{}

	mock.failGet = true
	_, err := s.manager.GetInstanceStatus(ctx, host)
	s.Error(err)

	active, err := s.manager.IsUp(ctx, host)
	s.Error(err)
	s.False(active)
}

func (s *OpenStackSuite) TestIsUpStatuses() {
	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.True(mock.isServerActive)

	host := &host.Host{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status, err := s.manager.GetInstanceStatus(ctx, host)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	active, err := s.manager.IsUp(ctx, host)
	s.NoError(err)
	s.True(active)

	mock.isServerActive = false
	status, err = s.manager.GetInstanceStatus(ctx, host)
	s.NoError(err)
	s.NotEqual(StatusRunning, status)

	active, err = s.manager.IsUp(ctx, host)
	s.NoError(err)
	s.False(active)
}

func (s *OpenStackSuite) TestTerminateInstanceAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostA := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(ctx, hostA)
	s.NotNil(hostA)
	s.NoError(err)
	_, err = hostA.Upsert()
	s.NoError(err)

	hostB := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(ctx, hostB)
	s.NotNil(hostB)
	s.NoError(err)
	_, err = hostB.Upsert()
	s.NoError(err)

	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(ctx, hostA, evergreen.User, ""))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(ctx, hostB, evergreen.User, ""))
}

func (s *OpenStackSuite) TestTerminateInstanceDB() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spawn the instance - check the host is not terminated in DB.
	myHost := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	myHost, err := s.manager.SpawnHost(ctx, myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert()
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotNil(dbHost)
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

func (s *OpenStackSuite) TestGetDNSNameAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock, ok := s.client.(*openStackClientMock)
	s.True(ok)
	s.False(mock.failGet)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(ctx, host)
	s.NoError(err)

	mock.failGet = true
	dns, err := s.manager.GetDNSName(ctx, host)
	s.Error(err)
	s.Empty(dns)
}

func (s *OpenStackSuite) TestSpawnInvalidSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	dProviderName := distro.Distro{Provider: evergreen.ProviderNameEc2Auto}
	h := host.NewIntent(dProviderName, dProviderName.GenerateName(), dProviderName.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	dSettingsNone := distro.Distro{Provider: evergreen.ProviderNameOpenstack}
	h = host.NewIntent(dSettingsNone, dSettingsNone.GenerateName(), dSettingsNone.Provider, s.hostOpts)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	dSettingsInvalid := distro.Distro{
		Provider:         evergreen.ProviderNameOpenstack,
		ProviderSettings: &map[string]interface{}{"image_name": ""},
	}
	h = host.NewIntent(dSettingsInvalid, dSettingsInvalid.GenerateName(), dSettingsInvalid.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *OpenStackSuite) TestSpawnDuplicateHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)

	hostOne, err := s.manager.SpawnHost(ctx, hostOne)
	s.NoError(err)
	s.NotNil(hostOne)
	_, err = hostOne.Upsert()
	s.NoError(err)

	hostTwo := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(ctx, hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
	_, err = hostTwo.Upsert()
	s.NoError(err)
}

func (s *OpenStackSuite) TestSpawnAPICall() {
	dist := distro.Distro{
		Id:       "id",
		Provider: evergreen.ProviderNameOpenstack,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := host.NewIntent(dist, dist.GenerateName(), dist.Provider, s.hostOpts)
	h, err := s.manager.SpawnHost(ctx, h)
	s.NoError(err)
	s.NotNil(h)
	_, err = h.Upsert()
	s.NoError(err)

	mock.failCreate = true
	h = host.NewIntent(dist, dist.GenerateName(), dist.Provider, s.hostOpts)

	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *OpenStackSuite) TestUtilToEvgStatus() {
	s.Equal(StatusRunning, osStatusToEvgStatus("ACTIVE"))
	s.Equal(StatusRunning, osStatusToEvgStatus("IN_PROGRESS"))
	s.Equal(StatusStopped, osStatusToEvgStatus("SHUTOFF"))
	s.Equal(StatusInitializing, osStatusToEvgStatus("BUILD"))
	s.Equal(StatusUnknown, osStatusToEvgStatus("???"))
}
