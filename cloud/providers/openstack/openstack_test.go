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
}

func (s *OpenStackSuite) TestValidateSettings() {
	settingsOk := &ProviderSettings{
		ImageName:  "image",
		FlavorName: "flavor",
	}
	s.NoError(settingsOk.Validate())

	settingsNoImage := &ProviderSettings{FlavorName: "flavor"}
	s.Error(settingsNoImage.Validate())

	settingsNoFlavor := &ProviderSettings{ImageName:  "image"}
	s.Error(settingsNoFlavor.Validate())
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
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failDelete)

	host := &host.Host{Id: "hostID"}
	s.NoError(s.manager.TerminateInstance(host))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(host))
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
	hostOpts := cloud.HostOptions{}

	dProviderName := &distro.Distro{Provider: "ec2"}
	host, err := s.manager.SpawnInstance(dProviderName, hostOpts)
	s.Error(err)
	s.Nil(host)

	dInvalidSettings := &distro.Distro{Provider: "OpenStack"}
	host, err = s.manager.SpawnInstance(dInvalidSettings, hostOpts)
	s.Error(err)
	s.Nil(host)
}

func (s *OpenStackSuite) TestSpawnAPICall() {
	dist := &distro.Distro{
		Id:       "id",
		Provider: "OpenStack",
		ProviderSettings: &map[string]interface{}{
			"ImageName":      "image",
			"FlavorName":     "flavor",
			"KeyName":        "key",
			"SecurityGroups": []string{"group"},
		},
	}
	opts := cloud.HostOptions{}

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failCreate)

	host, err := s.manager.SpawnInstance(dist, opts)
	s.NoError(err)
	s.NotNil(host)

	mock.failCreate = true
	host, err = s.manager.SpawnInstance(dist, opts)
	s.Error(err)
	s.Nil(host)
}
