package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/stretchr/testify/suite"
)

type AdminModelSuite struct {
	serviceSettings admin.AdminSettings
	apiSettings     APIAdminSettings
	restartResp     *RestartTasksResponse
	suite.Suite
}

func TestAdminModelSuite(t *testing.T) {
	suite.Run(t, new(AdminModelSuite))
}

func (s *AdminModelSuite) SetupSuite() {
	s.serviceSettings = admin.AdminSettings{
		Banner:      "banner text",
		BannerTheme: admin.Information,
		ServiceFlags: admin.ServiceFlags{
			TaskDispatchDisabled: true,
			MonitorDisabled:      true,
			TaskrunnerDisabled:   true,
		},
	}

	s.apiSettings = APIAdminSettings{
		Banner:      "banner text",
		BannerTheme: admin.Information,
		ServiceFlags: APIServiceFlags{
			TaskDispatchDisabled: true,
			MonitorDisabled:      true,
			TaskrunnerDisabled:   true,
		},
	}

	s.restartResp = &RestartTasksResponse{
		TasksRestarted: []string{"task1", "task2", "task3"},
		TasksErrored:   []string{"task4", "task5"},
	}
}

func (s *AdminModelSuite) TestBuildFromService() {
	// test that BuildFromService returns the correct model for valid input
	apiSettings := APIAdminSettings{}
	s.NoError(apiSettings.BuildFromService(&s.serviceSettings))
	s.Equal(s.apiSettings.Banner, apiSettings.Banner)
	s.Equal(s.apiSettings.ServiceFlags, apiSettings.ServiceFlags)

	apiBanner := APIBanner{}
	s.NoError(apiBanner.BuildFromService(APIBanner{
		Text:  APIString(s.serviceSettings.Banner),
		Theme: APIString(s.serviceSettings.BannerTheme),
	}))
	s.Equal(s.apiSettings.Banner, apiBanner.Text)
	s.Equal(s.apiSettings.BannerTheme, apiBanner.Theme)

	apiFlags := APIServiceFlags{}
	s.NoError(apiFlags.BuildFromService(s.serviceSettings.ServiceFlags))
	s.Equal(s.apiSettings.ServiceFlags, apiFlags)

	restartResp := RestartTasksResponse{}
	s.NoError(restartResp.BuildFromService(s.restartResp))
	s.Equal(3, len(restartResp.TasksRestarted))
	s.Equal(2, len(restartResp.TasksErrored))

	// test that BuildFromService errors for invalid input
	s.Error(apiSettings.BuildFromService(APIHost{}))
	s.Error(apiBanner.BuildFromService(APIHost{}))
	s.Error(apiFlags.BuildFromService(APIHost{}))
	s.Error(restartResp.BuildFromService(APIHost{}))
}

func (s *AdminModelSuite) TestToService() {
	// test that ToService returns the correct model for valid input
	serviceSettings, err := s.apiSettings.ToService()
	s.NoError(err)
	s.IsType(admin.AdminSettings{}, serviceSettings)
	adminSettings := serviceSettings.(admin.AdminSettings)
	s.Equal(s.serviceSettings.Banner, adminSettings.Banner)
	s.Equal(s.serviceSettings.BannerTheme, adminSettings.BannerTheme)
	s.Equal(s.serviceSettings.ServiceFlags, adminSettings.ServiceFlags)

	serviceFlags, err := s.apiSettings.ServiceFlags.ToService()
	s.NoError(err)
	s.IsType(admin.ServiceFlags{}, serviceFlags)
	s.Equal(s.serviceSettings.ServiceFlags, serviceFlags)
}
