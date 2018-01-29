package admin

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

var (
	testConfig = testutil.TestConfig()
)

type AdminSuite struct {
	suite.Suite
}

func TestAdminSuite(t *testing.T) {
	s := new(AdminSuite)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	db.Clear(Collection)

	suite.Run(t, s)
}

func (s *AdminSuite) TestBanner() {
	const bannerText = "hello evergreen users!"

	err := SetBanner(bannerText)
	s.NoError(err)
	settings, err := GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(bannerText, settings.Banner)

	err = SetBannerTheme(Important)
	s.NoError(err)
	settings, err = GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(Important, string(settings.BannerTheme))
}

func (s *AdminSuite) TestServiceFlags() {
	testFlags := ServiceFlags{
		TaskDispatchDisabled:         true,
		HostinitDisabled:             true,
		MonitorDisabled:              true,
		NotificationsDisabled:        true,
		AlertsDisabled:               true,
		TaskrunnerDisabled:           true,
		RepotrackerDisabled:          true,
		SchedulerDisabled:            true,
		GithubPRTestingDisabled:      true,
		RepotrackerPushEventDisabled: true,
		CLIUpdatesDisabled:           true,
		GithubStatusAPIDisabled:      true,
	}

	err := SetServiceFlags(testFlags)
	s.NoError(err)
	settings, err := GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(testFlags, settings.ServiceFlags)
}

func (s *AdminSuite) TestUpsert() {
	db.Clear(Collection)
	settings := &AdminSettings{
		Id:     systemSettingsDocID,
		Banner: "",
		ServiceFlags: ServiceFlags{
			TaskDispatchDisabled:         false,
			HostinitDisabled:             false,
			MonitorDisabled:              false,
			NotificationsDisabled:        false,
			AlertsDisabled:               false,
			TaskrunnerDisabled:           false,
			RepotrackerDisabled:          false,
			SchedulerDisabled:            false,
			GithubPRTestingDisabled:      false,
			RepotrackerPushEventDisabled: false,
			CLIUpdatesDisabled:           false,
			GithubStatusAPIDisabled:      false,
		},
		ConfigDir:          "ConfigDir",
		ApiUrl:             "ApiUrl",
		ClientBinariesDir:  "ClientBinariesDir",
		SuperUsers:         []string{"SuperUsers"},
		Jira:               evergreen.JiraConfig{Host: "foo"},
		Splunk:             send.SplunkConnectionInfo{Channel: "bar"},
		Slack:              evergreen.SlackConfig{Token: "token"},
		Providers:          evergreen.CloudProviders{AWS: evergreen.AWSConfig{Secret: "test"}},
		Keys:               map[string]string{"key1": "val1"},
		Credentials:        map[string]string{"key2": "val2"},
		AuthConfig:         evergreen.AuthConfig{Crowd: &evergreen.CrowdConfig{Username: "user"}},
		RepoTracker:        evergreen.RepoTrackerConfig{NumNewRepoRevisionsToFetch: 10},
		Api:                evergreen.APIConfig{HttpListenAddr: "HttpListenAddr"},
		Alerts:             evergreen.AlertsConfig{SMTP: &evergreen.SMTPConfig{Server: "server1", AdminEmail: []string{}}},
		Ui:                 evergreen.UIConfig{Url: "url"},
		HostInit:           evergreen.HostInitConfig{SSHTimeoutSeconds: 15},
		Notify:             evergreen.NotifyConfig{SMTP: &evergreen.SMTPConfig{Server: "server2", AdminEmail: []string{}}},
		Scheduler:          evergreen.SchedulerConfig{TaskFinder: "taskfinder"},
		Amboy:              evergreen.AmboyConfig{Name: "amboy"},
		Expansions:         map[string]string{"key3": "val3"},
		Plugins:            map[string]map[string]interface{}{"key4": map[string]interface{}{"keykey4": "val4"}},
		IsNonProd:          true,
		LoggerConfig:       evergreen.LoggerConfig{DefaultLevel: "level"},
		LogPath:            "logpath",
		PprofPort:          "port",
		GithubPRCreatorOrg: "github",
		NewRelic:           evergreen.NewRelicConfig{ApplicationName: "app"},
	}
	err := Upsert(settings)
	s.NoError(err)

	settingsFromDB, err := GetSettings()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings, settingsFromDB)

	settings.Banner = "test"
	settings.ServiceFlags.HostinitDisabled = true
	err = Upsert(settings)
	s.NoError(err)

	settingsFromDB, err = GetSettings()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings.Banner, settingsFromDB.Banner)
	s.Equal(settings.ServiceFlags.HostinitDisabled, settingsFromDB.ServiceFlags.HostinitDisabled)
}
