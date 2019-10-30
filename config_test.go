package evergreen

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	testDir      = "config_test"
	testSettings = "evg_settings.yml"
)

func testConfigFile() string {
	return filepath.Join(FindEvergreenHome(), testDir, testSettings)
}

// TestConfig creates test settings from a test config.
func testConfig() *Settings {
	settings, err := NewSettings(testConfigFile())
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

	token, err = settings.GetGithubOauthString()
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
	env Environment
	suite.Suite
}

func TestAdminSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configFile := os.Getenv("SETTINGS_OVERRIDE")
	if configFile == "" {
		configFile = testConfigFile()
	}
	env, err := NewEnvironment(ctx, configFile, nil)
	require.NoError(t, err)
	SetEnvironment(env)

	s := new(AdminSuite)
	s.env = env
	suite.Run(t, s)
}

func (s *AdminSuite) SetupTest() {
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
		Bugsnag:            "u-12345",
		ClientBinariesDir:  "bin_dir",
		ConfigDir:          "cfg_dir",
		Credentials:        map[string]string{"k1": "v1"},
		DomainName:         "example.com",
		Expansions:         map[string]string{"k2": "v2"},
		GithubPRCreatorOrg: "org",
		GithubOrgs:         []string{"evergreen-ci"},
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

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config.ApiUrl, settings.ApiUrl)
	s.Equal(config.Banner, settings.Banner)
	s.Equal(config.BannerTheme, settings.BannerTheme)
	s.Equal(config.Bugsnag, settings.Bugsnag)
	s.Equal(config.ClientBinariesDir, settings.ClientBinariesDir)
	s.Equal(config.ConfigDir, settings.ConfigDir)
	s.Equal(config.Credentials, settings.Credentials)
	s.Equal(config.DomainName, settings.DomainName)
	s.Equal(config.Expansions, settings.Expansions)
	s.Equal(config.GithubPRCreatorOrg, settings.GithubPRCreatorOrg)
	s.Equal(config.GithubOrgs, settings.GithubOrgs)
	s.Equal(config.Keys, settings.Keys)
	s.Equal(config.LogPath, settings.LogPath)
	s.Equal(config.Plugins, settings.Plugins)
	s.Equal(config.PprofPort, settings.PprofPort)
	s.Equal(config.SuperUsers, settings.SuperUsers)
}

func (s *AdminSuite) TestServiceFlags() {
	testFlags := ServiceFlags{}
	s.NotPanics(func() {
		v := reflect.ValueOf(&testFlags).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			f.SetBool(true)
		}
	}, "error setting all fields to true")

	err := testFlags.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(testFlags, settings.ServiceFlags)

	s.NotPanics(func() {
		t := reflect.TypeOf(&settings.ServiceFlags).Elem()
		v := reflect.ValueOf(&settings.ServiceFlags).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			s.True(f.Bool(), "all fields should be true, but '%s' was false", t.Field(i).Name)
		}
	})
}

func (s *AdminSuite) TestAlertsConfig() {
	config := AlertsConfig{
		SMTP: SMTPConfig{
			Server:     "server",
			Port:       2285,
			UseSSL:     true,
			Username:   "username",
			Password:   "password",
			From:       "from",
			AdminEmail: []string{"email"},
		},
	}

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Alerts)
}

func (s *AdminSuite) TestAmboyConfig() {
	config := AmboyConfig{
		Name:                                  "amboy",
		SingleName:                            "single",
		DB:                                    "db",
		PoolSizeLocal:                         10,
		PoolSizeRemote:                        20,
		LocalStorage:                          30,
		GroupDefaultWorkers:                   40,
		GroupBackgroundCreateFrequencyMinutes: 50,
		GroupPruneFrequencyMinutes:            60,
		GroupTTLMinutes:                       70,
	}

	err := config.Set()
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

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Api)
}

func (s *AdminSuite) TestAuthConfig() {
	config := AuthConfig{
		LDAP: &LDAPConfig{
			URL:                "url",
			Port:               "port",
			UserPath:           "path",
			ServicePath:        "bot",
			Group:              "group",
			ExpireAfterMinutes: "60",
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

	err := config.Set()
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

	err := config.Set()
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

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Jira)
}

func (s *AdminSuite) TestProvidersConfig() {
	config := CloudProviders{
		AWS: AWSConfig{
			EC2Secret: "aws_secret",
			EC2Key:    "aws",
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

	err := config.Set()
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

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.RepoTracker)
}

func (s *AdminSuite) TestSchedulerConfig() {
	config := SchedulerConfig{
		TaskFinder: "task_finder",
	}

	err := config.Set()
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

	err := config.Set()
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
		CsrfKey:        "csrf",
	}

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Ui)
}

func (s *AdminSuite) TestConfigDefaults() {
	config := testConfig()

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
	config.ExpansionsNew = util.KeyValuePairSlice{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	s.NoError(config.Validate())

	// spot check the defaults
	s.Equal("legacy", config.Scheduler.TaskFinder)
	s.Equal(defaultLogBufferingDuration, config.LoggerConfig.Buffer.DurationSeconds)
	s.Equal("info", config.LoggerConfig.DefaultLevel)
	s.Equal(defaultAmboyPoolSize, config.Amboy.PoolSizeLocal)
	s.Equal("v1", config.Expansions["k1"])
	s.Equal("v2", config.Expansions["k2"])
}

func (s *AdminSuite) TestKeyValPairsToMap() {
	config := Settings{
		ApiUrl:    "foo",
		ConfigDir: "foo",
		CredentialsNew: util.KeyValuePairSlice{
			{Key: "cred1key", Value: "cred1val"},
		},
		ExpansionsNew: util.KeyValuePairSlice{
			{Key: "exp1key", Value: "exp1val"},
		},
		KeysNew: util.KeyValuePairSlice{
			{Key: "key1key", Value: "key1val"},
		},
		PluginsNew: util.KeyValuePairSlice{
			{Key: "myPlugin", Value: util.KeyValuePairSlice{
				{Key: "pluginKey", Value: "pluginVal"},
			}},
		},
	}
	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set())
	dbConfig := Settings{}
	s.NoError(dbConfig.Get(s.env))
	s.Len(dbConfig.CredentialsNew, 1)
	s.Len(dbConfig.ExpansionsNew, 1)
	s.Len(dbConfig.KeysNew, 1)
	s.Len(dbConfig.PluginsNew, 1)
	s.Equal(config.CredentialsNew[0].Value, dbConfig.Credentials[config.CredentialsNew[0].Key])
	s.Equal(config.ExpansionsNew[0].Value, dbConfig.Expansions[config.ExpansionsNew[0].Key])
	s.Equal(config.KeysNew[0].Value, dbConfig.Keys[config.KeysNew[0].Key])
	pluginMap := dbConfig.Plugins[config.PluginsNew[0].Key]
	s.NotNil(pluginMap)
	s.Equal("pluginVal", pluginMap["pluginKey"])
}

func (s *AdminSuite) TestNotifyConfig() {
	config := NotifyConfig{
		SMTP: SMTPConfig{
			Server:     "server",
			Port:       2285,
			UseSSL:     true,
			Username:   "username",
			Password:   "password",
			From:       "from",
			AdminEmail: []string{"email"},
		},
	}

	err := config.Set()
	s.NoError(err)
	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Notify)

	config.BufferIntervalSeconds = 1
	config.BufferTargetPerInterval = 2
	s.NoError(config.Set())

	settings, err = GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Notify)
}

func (s *AdminSuite) TestContainerPoolsConfig() {

	invalidConfig := ContainerPoolsConfig{
		Pools: []ContainerPool{
			ContainerPool{
				Distro:        "d1",
				Id:            "test-pool-1",
				MaxContainers: -5,
			},
		},
	}

	err := invalidConfig.ValidateAndDefault()
	s.EqualError(err, "container pool field max_containers must be positive integer")

	validConfig := ContainerPoolsConfig{
		Pools: []ContainerPool{
			ContainerPool{
				Distro:        "d1",
				Id:            "test-pool-1",
				MaxContainers: 100,
			},
			ContainerPool{
				Distro:        "d2",
				Id:            "test-pool-2",
				MaxContainers: 1,
			},
		},
	}

	err = validConfig.Set()
	s.NoError(err)

	settings, err := GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(validConfig, settings.ContainerPools)

	validConfig.Pools[0].MaxContainers = 50
	s.NoError(validConfig.Set())

	settings, err = GetConfig()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(validConfig, settings.ContainerPools)

	lookup := settings.ContainerPools.GetContainerPool("test-pool-1")
	s.NotNil(lookup)
	s.Equal(*lookup, validConfig.Pools[0])

	lookup = settings.ContainerPools.GetContainerPool("test-pool-2")
	s.NotNil(lookup)
	s.Equal(*lookup, validConfig.Pools[1])

	lookup = settings.ContainerPools.GetContainerPool("test-pool-3")
	s.Nil(lookup)
}

func (s *AdminSuite) TestJIRANotificationsConfig() {
	c := JIRANotificationsConfig{
		CustomFields: JIRACustomFieldsByProject{
			{
				Project: "this",
				Fields: []JIRANotificationsCustomField{
					{
						Field:    "should",
						Template: "disappear",
					},
				},
			},
		},
	}
	s.NoError(c.Get(s.env))
	s.NotNil(c)
	s.Nil(c.CustomFields)
	s.NotPanics(func() {
		s.NoError(c.ValidateAndDefault())
	})

	c.CustomFields = JIRACustomFieldsByProject{
		{
			Project: "EVG",
			Fields: []JIRANotificationsCustomField{
				{
					Field:    "customfield_12345",
					Template: "magical{{.Template.Expansion}}",
				},
			},
		},
	}
	m, err := c.CustomFields.ToMap()
	s.NoError(err)
	s.Require().Len(m, 1)
	s.Require().Len(m["EVG"], 1)

	s.Require().NoError(c.Set())

	c = JIRANotificationsConfig{}
	s.Require().NoError(c.Get(s.env))
	s.NoError(c.ValidateAndDefault())
	m, err = c.CustomFields.ToMap()
	s.NoError(err)
	s.Require().Len(m, 1)
	s.Require().Len(m["EVG"], 1)
	s.Equal("magical{{.Template.Expansion}}", m["EVG"]["customfield_12345"])
	s.NoError(c.ValidateAndDefault())

	c = JIRANotificationsConfig{}
	s.NoError(c.Set())
	s.NoError(c.ValidateAndDefault())
	c.CustomFields = JIRACustomFieldsByProject{
		{
			Project: "this",
			Fields: []JIRANotificationsCustomField{
				{
					Field:    "is",
					Template: "{{.Invalid}",
				},
			},
		},
	}
	s.EqualError(c.ValidateAndDefault(), "template: this-is:1: unexpected \"}\" in operand")
}

func (s *AdminSuite) TestCommitQueueConfig() {
	config := CommitQueueConfig{
		MergeTaskDistro: "distro",
		CommitterName:   "Evergreen",
		CommitterEmail:  "evergreen@mongodb.com",
	}

	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set())

	settings, err := GetConfig()
	s.NoError(err)
	s.Require().NotNil(settings)

	s.Equal(config, settings.CommitQueue)
}

func (s *AdminSuite) TestHostJasperConfig() {
	emptyConfig := HostJasperConfig{}
	s.NoError(emptyConfig.ValidateAndDefault())
	s.Equal(DefaultJasperPort, emptyConfig.Port)

	config := HostJasperConfig{
		BinaryName:       "foo",
		DownloadFileName: "bar",
		Port:             12345,
		URL:              "bat",
		Version:          "baz",
	}

	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set())

	settings, err := GetConfig()
	s.Require().NoError(err)

	s.Equal(config, settings.HostJasper)
}
