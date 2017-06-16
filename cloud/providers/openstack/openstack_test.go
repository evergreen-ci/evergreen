package openstack

import (
	"testing"

	"github.com/evergreen-ci/evergreen"

	"github.com/stretchr/testify/suite"
	"github.com/gophercloud/gophercloud"
)

type OpenStackSuite struct {
	authOptions  *gophercloud.AuthOptions
	endpointOpts *gophercloud.EndpointOpts
	client       client
	manager      *Manager
	suite.Suite
}

func TestOpenStackSuite(t *testing.T) {
	suite.Run(t, new(OpenStackSuite))
}

func (s *OpenStackSuite) SetupSuite() {}

func (s *OpenStackSuite) SetupTest() {
	s.authOptions  = &gophercloud.AuthOptions{}
	s.endpointOpts = &gophercloud.EndpointOpts{}
	s.client       = &clientMock{}

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
