package evergreen

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	testDir      = "config_test"
	testSettings = "evg_settings.yml"
)

// TestConfig creates test settings from a test config.
func testConfig() *Settings {
	file := filepath.Join(FindEvergreenHome(), testDir, testSettings)
	settings, err := NewSettings(file)
	if err != nil {
		panic(err)
	}

	if err = settings.Validate(); err != nil {
		panic(err)
	}

	return settings
}

//Checks that the test settings file can be parsed
//and returns a settings object.
func TestInitSettings(t *testing.T) {
	assert := assert.New(t)

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "mci_settings.yml"))
	assert.NoError(err, "Parsing a valid settings file should succeed")
	assert.NotNil(settings)
}

//Checks that trying to parse a non existent file returns non-nil err
func TestBadInit(t *testing.T) {
	assert := assert.New(t)

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "blahblah.yml"))

	assert.Error(err, "Parsing a nonexistent config file should cause an error")
	assert.Nil(settings)
}

func TestGetGithubSettings(t *testing.T) {
	assert := assert.New(t)

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "mci_settings.yml"))
	assert.NoError(err)
	assert.Empty(settings.Credentials["github"])

	token, err := settings.GetGithubOauthToken()
	assert.Error(err)
	assert.Empty(token)

	settings, err = NewSettings(filepath.Join(FindEvergreenHome(),
		"config_test", "evg_settings.yml"))
	assert.NoError(err)
	assert.NotNil(settings.Credentials["github"])

	token, err = settings.GetGithubOauthToken()
	assert.NoError(err)
	assert.Equal(settings.Credentials["github"], token)

	assert.NotPanics(func() {
		settings := &Settings{}
		assert.Nil(settings.Credentials)

		token, err = settings.GetGithubOauthToken()
		assert.Error(err)
		assert.Empty(token)
	})
}

type AdminSuite struct {
	suite.Suite
}

func TestAdminSuite(t *testing.T) {
	s := new(AdminSuite)
	config := testConfig()
	db.SetGlobalSessionProvider(config.SessionFactory())
	suite.Run(t, s)
}

func (s *AdminSuite) SetupTest() {
	s.NoError(db.Clear(ConfigCollection))
	s.NoError(resetRegistry())
}

func (s *AdminSuite) TestBanner() {
	const bannerText = "hello evergreen users!"

	err := SetBanner(bannerText)
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(bannerText, settings.Banner)

	err = SetBannerTheme(Important)
	s.NoError(err)
	settings, err = GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(Important, string(settings.BannerTheme))
}

func (s *AdminSuite) TestBaseConfig() {
	config := Settings{
		ApiUrl:             "api",
		Banner:             "banner",
		BannerTheme:        Important,
		ClientBinariesDir:  "bin_dir",
		ConfigDir:          "cfg_dir",
		Credentials:        map[string]string{"k1": "v1"},
		Expansions:         map[string]string{"k2": "v2"},
		GithubPRCreatorOrg: "org",
		IsNonProd:          true,
		Keys:               map[string]string{"k3": "v3"},
		LogPath:            "logpath",
		Plugins:            map[string]map[string]interface{}{"k4": map[string]interface{}{"k5": "v5"}},
		PprofPort:          "port",
		Splunk: send.SplunkConnectionInfo{
			ServerURL: "server",
			Token:     "token",
			Channel:   "channel",
		},
		SuperUsers: []string{"user"},
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config.ApiUrl, settings.ApiUrl)
	s.Equal(config.Banner, settings.Banner)
	s.Equal(config.BannerTheme, settings.BannerTheme)
	s.Equal(config.ClientBinariesDir, settings.ClientBinariesDir)
	s.Equal(config.ConfigDir, settings.ConfigDir)
	s.Equal(config.Credentials, settings.Credentials)
	s.Equal(config.Expansions, settings.Expansions)
	s.Equal(config.GithubPRCreatorOrg, settings.GithubPRCreatorOrg)
	s.Equal(config.IsNonProd, settings.IsNonProd)
	s.Equal(config.Keys, settings.Keys)
	s.Equal(config.LogPath, settings.LogPath)
	s.Equal(config.Plugins, settings.Plugins)
	s.Equal(config.PprofPort, settings.PprofPort)
	s.Equal(config.SuperUsers, settings.SuperUsers)
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

	err := testFlags.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(testFlags, settings.ServiceFlags)
}

func (s *AdminSuite) TestAlertsConfig() {
	config := AlertsConfig{
		SMTP: &SMTPConfig{
			Server:     "server",
			Port:       2285,
			UseSSL:     true,
			Username:   "username",
			Password:   "password",
			From:       "from",
			AdminEmail: []string{"email"},
		},
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Alerts)
}

func (s *AdminSuite) TestAmboyConfig() {
	config := AmboyConfig{
		Name:           "amboy",
		DB:             "db",
		PoolSizeLocal:  10,
		PoolSizeRemote: 20,
		LocalStorage:   30,
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Amboy)
}

func (s *AdminSuite) TestApiConfig() {
	config := APIConfig{
		HttpListenAddr:      "addr",
		GithubWebhookSecret: "secret",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Api)
}

func (s *AdminSuite) TestAuthConfig() {
	config := AuthConfig{
		Crowd: &CrowdConfig{
			Username: "crowduser",
			Password: "crowdpw",
			Urlroot:  "crowdurl",
		},
		Naive: &NaiveAuthConfig{
			Users: []*AuthUser{&AuthUser{Username: "user", Password: "pw"}},
		},
		Github: &GithubAuthConfig{
			ClientId:     "ghclient",
			ClientSecret: "ghsecret",
			Users:        []string{"ghuser"},
			Organization: "ghorg",
		},
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.AuthConfig)
}

func (s *AdminSuite) TestHostinitConfig() {
	config := HostInitConfig{
		SSHTimeoutSeconds: 10,
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.HostInit)
}

func (s *AdminSuite) TestJiraConfig() {
	config := JiraConfig{
		Host:           "host",
		Username:       "username",
		Password:       "password",
		DefaultProject: "proj",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Jira)
}

func (s *AdminSuite) TestNewRelicConfig() {
	config := NewRelicConfig{
		ApplicationName: "new_relic",
		LicenseKey:      "key",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.NewRelic)
}

func (s *AdminSuite) TestNotifyConfig() {
	config := NotifyConfig{
		SMTP: &SMTPConfig{
			Server:     "server",
			Port:       2285,
			UseSSL:     true,
			Username:   "username",
			Password:   "password",
			From:       "from",
			AdminEmail: []string{"email"},
		},
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Notify)
}

func (s *AdminSuite) TestProvidersConfig() {
	config := CloudProviders{
		AWS: AWSConfig{
			Secret: "aws_secret",
			Id:     "aws",
		},
		Docker: DockerConfig{
			APIVersion: "docker_version",
		},
		GCE: GCEConfig{
			ClientEmail:  "gce_email",
			PrivateKey:   "gce_key",
			PrivateKeyID: "gce_key_id",
			TokenURI:     "gce_token",
		},
		OpenStack: OpenStackConfig{
			IdentityEndpoint: "endpoint",
			Username:         "username",
			Password:         "password",
			DomainName:       "domain",
			ProjectName:      "project",
			ProjectID:        "project_id",
			Region:           "region",
		},
		VSphere: VSphereConfig{
			Host:     "host",
			Username: "vsphere",
			Password: "vsphere_pass",
		},
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Providers)
}

func (s *AdminSuite) TestRepotrackerConfig() {
	config := RepoTrackerConfig{
		NumNewRepoRevisionsToFetch: 10,
		MaxRepoRevisionsToSearch:   20,
		MaxConcurrentRequests:      30,
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.RepoTracker)
}

func (s *AdminSuite) TestSchedulerConfig() {
	config := SchedulerConfig{
		MergeToggle: 10,
		TaskFinder:  "task_finder",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Scheduler)
}

func (s *AdminSuite) TestSlackConfig() {
	config := SlackConfig{
		Options: &send.SlackOptions{
			Channel:   "channel",
			Fields:    true,
			FieldsSet: map[string]bool{},
		},
		Token: "token",
		Level: "info",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Slack)
}

func (s *AdminSuite) TestUiConfig() {
	config := UIConfig{
		Url:            "url",
		HelpUrl:        "helpurl",
		HttpListenAddr: "addr",
		Secret:         "secret",
		DefaultProject: "mci",
		CacheTemplates: true,
		SecureCookies:  true,
		CsrfKey:        "csrf",
	}

	err := config.set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Ui)
}

func (s *AdminSuite) TestConfigDefaults() {
	config, err := GetConfig()
	s.NoError(err)
	s.Require().NotNil(config)
	config.Database = DBSettings{
		Url: "url",
		DB:  "db",
	}
	config.AuthConfig = AuthConfig{
		Naive: &NaiveAuthConfig{},
	}
	config.Ui = UIConfig{
		Secret:         "secret",
		DefaultProject: "proj",
		Url:            "url",
	}
	config.ApiUrl = "api"
	config.ConfigDir = "dir"
	s.NoError(config.Validate())

	// spot check the defaults
	s.Nil(config.Notify.SMTP)
	s.Equal("legacy", config.Scheduler.TaskFinder)
	s.Equal(defaultLogBufferingDuration, config.LoggerConfig.Buffer.DurationSeconds)
	s.Equal("info", config.LoggerConfig.DefaultLevel)
	s.Equal(defaultAmboyPoolSize, config.Amboy.PoolSizeLocal)
}
