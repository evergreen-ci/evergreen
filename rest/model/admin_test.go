package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/stretchr/testify/suite"
)

type AdminModelSuite struct {
	serviceSettings admin.AdminSettings
	apiSettings     APIAdminSettings
	suite.Suite
}

func TestAdminModelSuite(t *testing.T) {
	suite.Run(t, new(AdminModelSuite))
}

func (s *AdminModelSuite) SetupSuite() {
	s.serviceSettings = admin.AdminSettings{
		Banner: "banner text",
		ServiceFlags: admin.ServiceFlags{
			TaskDispatchDisabled: true,
			MonitorDisabled:      true,
			TaskrunnerDisabled:   true,
		},
	}

	s.apiSettings = APIAdminSettings{
		Banner: "banner text",
		ServiceFlags: APIServiceFlags{
			TaskDispatchDisabled: true,
			MonitorDisabled:      true,
			TaskrunnerDisabled:   true,
		},
	}
}

func (s *AdminModelSuite) TestBuildFromService() {
	// test that BuildFromService returns the correct model for valid input
	apiSettings := APIAdminSettings{}
	s.NoError(apiSettings.BuildFromService(&s.serviceSettings))
	s.Equal(s.apiSettings.Banner, apiSettings.Banner)
	s.Equal(s.apiSettings.ServiceFlags, apiSettings.ServiceFlags)

	apiBanner := APIBanner{}
	s.NoError(apiBanner.BuildFromService(s.serviceSettings.Banner))
	s.Equal(s.apiSettings.Banner, apiBanner.Text)

	apiFlags := APIServiceFlags{}
	s.NoError(apiFlags.BuildFromService(s.serviceSettings.ServiceFlags))
	s.Equal(s.apiSettings.ServiceFlags, apiFlags)

	// test that BuildFromService errors for invalid input
	s.Error(apiSettings.BuildFromService(APIHost{}))
	s.Error(apiBanner.BuildFromService(APIHost{}))
	s.Error(apiFlags.BuildFromService(APIHost{}))
}

func (s *AdminModelSuite) TestToService() {
	// test that ToService returns the correct model for valid input
	serviceSettings, err := s.apiSettings.ToService()
	s.NoError(err)
	s.IsType(admin.AdminSettings{}, serviceSettings)
	adminSettings := serviceSettings.(admin.AdminSettings)
	s.Equal(s.serviceSettings.Banner, adminSettings.Banner)
	s.Equal(s.serviceSettings.ServiceFlags, adminSettings.ServiceFlags)

	serviceFlags, err := s.apiSettings.ServiceFlags.ToService()
	s.NoError(err)
	s.IsType(admin.ServiceFlags{}, serviceFlags)
	s.Equal(s.serviceSettings.ServiceFlags, serviceFlags)
}
