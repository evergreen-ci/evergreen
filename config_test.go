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
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	testDir = "config_test"
	// testSettings contains the default admin settings suitable for testing
	// that depends on the global environment.
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

// Checks that the test settings file can be parsed
// and returns a settings object.
func TestInitSettings(t *testing.T) {
	assert := assert.New(t)

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "mci_settings.yml"))
	assert.NoError(err, "Parsing a valid settings file should succeed")
	assert.NotNil(settings)
}

// Checks that trying to parse a non existent file returns non-nil err
func TestBadInit(t *testing.T) {
	assert := assert.New(t)

	settings, err := NewSettings(filepath.Join(FindEvergreenHome(),
		"testdata", "blahblah.yml"))

	assert.Error(err, "Parsing a nonexistent config file should cause an error")
	assert.Nil(settings)
}

type AdminSuite struct {
	env              Environment
	originalEnv      Environment
	originalSettings *Settings
	suite.Suite
}

func TestAdminSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configFile := os.Getenv(SettingsOverride)
	if configFile == "" {
		configFile = testConfigFile()
	}

	originalEnv := GetEnvironment()
	originalSettings, err := NewSettings(configFile)
	require.NoError(t, err)
	env, err := NewEnvironment(ctx, configFile, "", "", nil, noop.NewTracerProvider())
	require.NoError(t, err)

	s := new(AdminSuite)
	s.env = env
	s.originalEnv = originalEnv
	s.originalSettings = originalSettings
	suite.Run(t, s)
}

func (s *AdminSuite) SetupTest() {
	SetEnvironment(s.env)
}

func (s *AdminSuite) TearDownTest() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Reset the global env and admin settings after modifications to avoid
	// affecting other tests that depend on the global test env.
	SetEnvironment(s.originalEnv)
	s.NoError(UpdateConfig(ctx, s.originalSettings))
}

func (s *AdminSuite) TestBanner() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const bannerText = "hello evergreen users!"

	err := SetBanner(ctx, bannerText)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(bannerText, settings.Banner)

	err = SetBannerTheme(ctx, Important)
	s.NoError(err)
	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(Important, settings.BannerTheme)
}

func (s *AdminSuite) TestBaseConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This test does not check Expansions because it is not possible to
	// call parameter store functions, real or mocked, in this test suite.
	config := Settings{
		AWSInstanceRole:     "role",
		Banner:              "banner",
		BannerTheme:         Important,
		ConfigDir:           "cfg_dir",
		DomainName:          "example.com",
		GithubPRCreatorOrg:  "org",
		GithubOrgs:          []string{"evergreen-ci"},
		LogPath:             "logpath",
		Plugins:             map[string]map[string]any{"k4": {"k5": "v5"}},
		PprofPort:           "port",
		ShutdownWaitSeconds: 15,
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config.AWSInstanceRole, settings.AWSInstanceRole)
	s.Equal(config.Banner, settings.Banner)
	s.Equal(config.BannerTheme, settings.BannerTheme)
	s.Equal(config.ConfigDir, settings.ConfigDir)
	s.Equal(config.DomainName, settings.DomainName)
	s.Equal(config.GithubPRCreatorOrg, settings.GithubPRCreatorOrg)
	s.Equal(config.GithubOrgs, settings.GithubOrgs)
	s.Equal(config.LogPath, settings.LogPath)
	s.Equal(config.Plugins, settings.Plugins)
	s.Equal(config.PprofPort, settings.PprofPort)
	s.Equal(config.ShutdownWaitSeconds, settings.ShutdownWaitSeconds)
}

func (s *AdminSuite) TestServiceFlags() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testFlags := ServiceFlags{}
	s.NotPanics(func() {
		v := reflect.ValueOf(&testFlags).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			f.SetBool(true)
		}
	})

	err := testFlags.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
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

func (s *AdminSuite) TestAmboyConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := AmboyConfig{
		Name:                                  "amboy",
		SingleName:                            "single",
		PoolSizeLocal:                         10,
		PoolSizeRemote:                        20,
		LocalStorage:                          30,
		GroupDefaultWorkers:                   40,
		GroupBackgroundCreateFrequencyMinutes: 50,
		GroupPruneFrequencyMinutes:            60,
		GroupTTLMinutes:                       70,
		LockTimeoutMinutes:                    7,
		SampleSize:                            200,
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Amboy)
}

func (s *AdminSuite) TestAdminDBConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := AmboyDBConfig{
		URL:      "mongodb://localhost:27017",
		Database: "db",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.AmboyDB)
}

func (s *AdminSuite) TestApiConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := APIConfig{
		HttpListenAddr: "addr",
		URL:            "api",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Api)
}

func (s *AdminSuite) TestAuthConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := AuthConfig{
		Okta: &OktaConfig{
			ClientID:           "id",
			ClientSecret:       "secret",
			Issuer:             "issuer",
			Scopes:             []string{"openid", "email", "profile", "offline_access"},
			UserGroup:          "group",
			ExpireAfterMinutes: 60,
		},
		Naive: &NaiveAuthConfig{
			Users: []AuthUser{{Username: "user", Password: "pw"}},
		},
		Github: &GithubAuthConfig{
			ClientId:     "ghclient",
			ClientSecret: "ghsecret",
			Users:        []string{"ghuser"},
			Organization: "ghorg",
			AppId:        1234,
		},
		Multi: &MultiAuthConfig{
			ReadWrite: []string{AuthGithubKey},
		},
		Kanopy: &KanopyAuthConfig{
			HeaderName: "internal_header",
		},
		BackgroundReauthMinutes: 60,
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.AuthConfig)
}

func (s *AdminSuite) TestHostinitConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := HostInitConfig{
		HostThrottle: 64,
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.HostInit)
}

func (s *AdminSuite) TestJiraConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := JiraConfig{
		Host:                "host",
		PersonalAccessToken: "personal_access_token",
		Email:               "a@mail.com",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Jira)
}

func (s *AdminSuite) TestParameterStoreConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := ParameterStoreConfig{
		Prefix: "/evergreen",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.ParameterStore)
}

func (s *AdminSuite) TestProvidersConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := CloudProviders{
		AWS: AWSConfig{
			EC2Keys: []EC2Key{
				{
					Secret: "aws_secret",
					Key:    "aws",
				},
			},
		},
		Docker: DockerConfig{
			APIVersion: "docker_version",
		},
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Providers)
}

func (s *AdminSuite) TestRepotrackerConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := RepoTrackerConfig{
		NumNewRepoRevisionsToFetch: 10,
		MaxRepoRevisionsToSearch:   20,
		MaxConcurrentRequests:      30,
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.RepoTracker)
}

func (s *AdminSuite) TestSchedulerConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := SchedulerConfig{
		TaskFinder: "task_finder",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Scheduler)
}

func (s *AdminSuite) TestSlackConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := SlackConfig{
		Options: &send.SlackOptions{
			Channel:   "channel",
			Fields:    true,
			FieldsSet: map[string]bool{},
		},
		Token: "token",
		Level: "info",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Slack)
}

func (s *AdminSuite) TestSplunkConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := SplunkConfig{
		SplunkConnectionInfo: send.SplunkConnectionInfo{
			ServerURL: "splunk_url",
			Token:     "splunk_token",
			Channel:   "splunk_channel",
		},
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Splunk)
}

func (s *AdminSuite) TestUiConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := UIConfig{
		Url:                "url",
		HttpListenAddr:     "addr",
		Secret:             "secret",
		DefaultProject:     "mci",
		CacheTemplates:     true,
		CsrfKey:            "csrf",
		StagingEnvironment: "mine",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := Settings{
		ConfigDir: "foo",
		ExpansionsNew: util.KeyValuePairSlice{
			{Key: "exp1key", Value: "exp1val"},
		},
		PluginsNew: util.KeyValuePairSlice{
			{Key: "myPlugin", Value: util.KeyValuePairSlice{
				{Key: "pluginKey", Value: "pluginVal"},
			}},
		},
	}
	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set(ctx))
	dbConfig := Settings{}
	s.NoError(dbConfig.Get(ctx))
	s.Len(dbConfig.ExpansionsNew, 1)
	s.Len(dbConfig.PluginsNew, 1)
	s.Equal(config.ExpansionsNew[0].Value, dbConfig.Expansions[config.ExpansionsNew[0].Key])
	pluginMap := dbConfig.Plugins[config.PluginsNew[0].Key]
	s.NotNil(pluginMap)
	s.Equal("pluginVal", pluginMap["pluginKey"])
}

func (s *AdminSuite) TestExpansionValidation() {
	config := Settings{
		ConfigDir: "dir",
		Expansions: map[string]string{
			"validKey": "validValue",
			"emptyKey": "",
		},
	}

	err := config.ValidateAndDefault()
	s.Error(err)
	s.Contains(err.Error(), "expansion 'emptyKey' cannot have an empty value")
}

func (s *AdminSuite) TestNotifyConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := NotifyConfig{
		SES: SESConfig{
			SenderAddress: "from",
		},
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Notify)

	config.BufferIntervalSeconds = 1
	config.BufferTargetPerInterval = 2
	s.NoError(config.Set(ctx))

	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Notify)
}

func (s *AdminSuite) TestLoggerConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := LoggerConfig{
		RedactKeys: []string{
			"secret",
			"github token",
		},
	}
	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.LoggerConfig)
}

func (s *AdminSuite) TestContainerPoolsConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invalidConfig := ContainerPoolsConfig{
		Pools: []ContainerPool{
			{
				Distro:        "d1",
				Id:            "test-pool-1",
				MaxContainers: -5,
			},
		},
	}

	err := invalidConfig.ValidateAndDefault()
	s.EqualError(err, "container pool max containers must be positive integer")

	validConfig := ContainerPoolsConfig{
		Pools: []ContainerPool{
			{
				Distro:        "d1",
				Id:            "test-pool-1",
				MaxContainers: 100,
			},
			{
				Distro:        "d2",
				Id:            "test-pool-2",
				MaxContainers: 1,
			},
		},
	}

	err = validConfig.Set(ctx)
	s.NoError(err)

	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(validConfig, settings.ContainerPools)

	validConfig.Pools[0].MaxContainers = 50
	s.NoError(validConfig.Set(ctx))

	settings, err = GetConfig(ctx)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := JIRANotificationsConfig{
		CustomFields: []JIRANotificationsProject{
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
	s.NoError(c.Get(ctx))
	s.NotNil(c)
	s.Nil(c.CustomFields)
	s.NotPanics(func() {
		s.NoError(c.ValidateAndDefault())
	})

	c.CustomFields = []JIRANotificationsProject{
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
	s.Require().NoError(c.Set(ctx))

	c = JIRANotificationsConfig{}
	s.Require().NoError(c.Get(ctx))
	s.NoError(c.ValidateAndDefault())
	s.Len(c.CustomFields, 1)

	c = JIRANotificationsConfig{}
	s.NoError(c.Set(ctx))
	s.NoError(c.ValidateAndDefault())
	c.CustomFields = []JIRANotificationsProject{
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
	s.EqualError(c.ValidateAndDefault(), "template: this-is:1: bad character U+007D '}'")
}

func (s *AdminSuite) TestHostJasperConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	s.NoError(config.Set(ctx))

	settings, err := GetConfig(ctx)
	s.Require().NoError(err)

	s.Equal(config, settings.HostJasper)
}

func (s *AdminSuite) TestSleepScheduleConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emptyConfig := SleepScheduleConfig{}
	s.NoError(emptyConfig.ValidateAndDefault())

	config := SleepScheduleConfig{
		PermanentlyExemptHosts: []string{"host0", "host1"},
	}

	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set(ctx))

	settings, err := GetConfig(ctx)
	s.Require().NoError(err)

	s.Equal(config, settings.SleepSchedule)
}

func (s *AdminSuite) TestCedarConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := CedarConfig{
		DBURL:  "url.com",
		DBName: "username",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Cedar)

	s.NoError(config.Set(ctx))

	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Cedar)
}

func (s *AdminSuite) TestSSHConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := SSHConfig{
		TaskHostKey: SSHKeyPair{
			Name:      "task-host-key",
			SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/top-secret-private-key",
		},
		SpawnHostKey: SSHKeyPair{
			Name:      "spawn-host-key",
			SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/confidential-private-key",
		},
	}

	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set(ctx))

	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.Require().NotNil(settings)

	s.Equal(config, settings.SSH)
}

func (s *AdminSuite) TestTracerConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := TracerConfig{
		Enabled:                   true,
		CollectorEndpoint:         "localhost:4316",
		CollectorInternalEndpoint: "svc.cluster.local:4317",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Tracer)

	config.Enabled = false
	s.NoError(config.Set(ctx))

	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Tracer)
}

func (s *AdminSuite) TestOverridesConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := OverridesConfig{
		Overrides: []Override{
			{
				SectionID: "test-section",
				Field:     "test-field",
				Value:     "test-value",
			},
		},
	}

	s.NoError(config.ValidateAndDefault())
	s.NoError(config.Set(ctx))

	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.Require().NotNil(settings)

	s.Equal(config, settings.Overrides)
}

func (s *AdminSuite) TestBucketsConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test basic bucket config
	config := BucketsConfig{
		LogBucket: BucketConfig{
			Name: "logs",
			Type: "s3",
		},
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Buckets)

	config.LogBucket.Name = "logs-2"
	s.NoError(config.Set(ctx))

	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Buckets)

	// Test long retention bucket and projects
	config.LogBucketLongRetention = BucketConfig{
		Name: "logs-long-retention",
		Type: "s3",
	}
	config.LongRetentionProjects = []string{"project1", "project2"}

	err = config.Set(ctx)
	s.NoError(err)
	settings, err = GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Buckets)
	s.Len(settings.Buckets.LongRetentionProjects, 2)

	config.LogBucketFailedTasks = BucketConfig{
		Name: "logs-failed-tasks",
		Type: BucketTypeS3,
	}
	err = config.Set(ctx)
	s.NoError(err, "Set should not error for LogBucketFailedTasks")
	settings, err = GetConfig(ctx)
	s.NoError(err, "GetConfig should not error after setting LogBucketFailedTasks")
	s.NotNil(settings, "Settings should not be nil after setting LogBucketFailedTasks")
	s.Equal(config.LogBucketFailedTasks, settings.Buckets.LogBucketFailedTasks, "LogBucketFailedTasks should match config")

	s.Equal("logs-failed-tasks", settings.Buckets.LogBucketFailedTasks.Name, "LogBucketFailedTasks.Name should be 'logs-failed-tasks'")
	s.Equal(BucketTypeS3, settings.Buckets.LogBucketFailedTasks.Type, "LogBucketFailedTasks.Type should be 's3'")

	// Test validation
	err = config.ValidateAndDefault()
	s.NoError(err)

	// Test GetLogBucket method
	longRetentionBucket := settings.Buckets.GetLogBucket("project1")
	s.Equal("logs-long-retention", longRetentionBucket.Name)
	s.Equal(BucketTypeS3, longRetentionBucket.Type)

	longRetentionBucket2 := settings.Buckets.GetLogBucket("project2")
	s.Equal("logs-long-retention", longRetentionBucket2.Name)

	defaultBucket := settings.Buckets.GetLogBucket("other-project")
	s.Equal("logs-2", defaultBucket.Name)
	s.Equal(BucketTypeS3, defaultBucket.Type)

	// Test invalid bucket type
	config.LogBucketLongRetention.Type = "invalid"
	err = config.ValidateAndDefault()
	s.Error(err)
	s.Contains(err.Error(), "unrecognized bucket type 'invalid'")
}

func (s *AdminSuite) TestSageConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := SageConfig{
		BaseURL: "https://sage.example.com/api",
	}

	err := config.Set(ctx)
	s.NoError(err)
	settings, err := GetConfig(ctx)
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(config, settings.Sage)
}
