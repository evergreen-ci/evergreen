package testutil

import (
	"context"
	"flag"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

var ExecutionEnvironmentType = "production"

const (
	TestDir = "config_test"
	// TestSettings contains the default admin settings suitable for testing
	// that depends on the global environment.
	TestSettings = "evg_settings.yml"
)

func init() {
	if ExecutionEnvironmentType != "test" {
		grip.Alert(message.Fields{
			"op":      "called init() in testutil for production code.",
			"test.v":  flag.Lookup("test.v"),
			"v":       flag.Lookup("v"),
			"args":    flag.Args(),
			"environ": ExecutionEnvironmentType,
		})
	} else {
		Setup()
	}
}

func Setup() {
	if evergreen.GetEnvironment() == nil {
		ctx := context.Background()

		path := filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings)
		env, err := evergreen.NewEnvironment(ctx, path, "", "", nil, noop.NewTracerProvider())

		grip.EmergencyPanic(message.WrapError(err, message.Fields{
			"message": "could not initialize test environment",
			"path":    filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings),
		}))

		// For testing purposes, set up parameter manager so it's backed by the
		// DB.
		pm, err := parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
			PathPrefix:     env.Settings().ParameterStore.Prefix,
			CachingEnabled: true,
			SSMClient:      fakeparameter.NewFakeSSMClient(),
			DB:             env.DB(),
		})
		grip.EmergencyPanic(message.WrapError(err, message.Fields{
			"message": "could not initialize test environment's parameter manager",
		}))

		env.SetParameterManager(pm)

		evergreen.SetEnvironment(env)
	}
}

func NewEnvironment(ctx context.Context, t *testing.T) evergreen.Environment {
	env, err := evergreen.NewEnvironment(ctx, filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings), "", "", nil, noop.NewTracerProvider())
	require.NoError(t, err)
	// For testing purposes, set up parameter manager so it's backed by the DB.
	pm, err := parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
		PathPrefix:     env.Settings().ParameterStore.Prefix,
		CachingEnabled: true,
		SSMClient:      fakeparameter.NewFakeSSMClient(),
		DB:             env.DB(),
	})
	require.NoError(t, err)
	env.SetParameterManager(pm)
	return env
}

// TestConfig creates test settings from a test config.
func TestConfig() *evergreen.Settings {
	return loadConfig(TestDir, TestSettings)
}

func loadConfig(path ...string) *evergreen.Settings {
	paths := []string{evergreen.FindEvergreenHome()}
	paths = append(paths, path...)
	settings, err := evergreen.NewSettings(filepath.Join(paths...))
	if err != nil {
		panic(err)
	}

	if err = settings.Validate(); err != nil {
		panic(err)
	}

	return settings
}

func MockConfig() *evergreen.Settings {
	return &evergreen.Settings{
		Amboy: evergreen.AmboyConfig{
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
			SampleSize:                            500,
			Retry: evergreen.AmboyRetryConfig{
				NumWorkers:                          8,
				MaxCapacity:                         100,
				MaxRetryAttempts:                    50,
				MaxRetryTimeSeconds:                 60,
				RetryBackoffSeconds:                 10,
				StaleRetryingMonitorIntervalSeconds: 30,
			},
			NamedQueues: []evergreen.AmboyNamedQueueConfig{
				{
					Name:               "queue0",
					NumWorkers:         5,
					SampleSize:         100,
					LockTimeoutSeconds: 80,
				},
				{
					Name:       "queue1",
					SampleSize: 500,
				},
				{
					Regexp:             "^queue2",
					LockTimeoutSeconds: 50,
				},
			},
		},
		AmboyDB: evergreen.AmboyDBConfig{
			Database: "db",
			URL:      "mongodb://localhost:27017",
		},
		Api: evergreen.APIConfig{
			HttpListenAddr: "addr",
			URL:            "api",
			CorpURL:        "corp",
		},
		AuthConfig: evergreen.AuthConfig{
			Okta: &evergreen.OktaConfig{
				ClientID:           "id",
				ClientSecret:       "secret",
				Issuer:             "issuer",
				Scopes:             []string{"openid", "email", "profile", "offline_access"},
				UserGroup:          "group",
				ExpireAfterMinutes: 60,
			},
			Naive: &evergreen.NaiveAuthConfig{
				Users: []evergreen.AuthUser{{Username: "user", Password: "pw"}},
			},
			Github: &evergreen.GithubAuthConfig{
				ClientId:     "ghclient",
				ClientSecret: "ghsecret",
				Users:        []string{"ghuser"},
				Organization: "ghorg",
			},
			Multi: &evergreen.MultiAuthConfig{
				ReadOnly:  []string{evergreen.AuthNaiveKey},
				ReadWrite: []string{evergreen.AuthOktaKey},
			},
			Kanopy: &evergreen.KanopyAuthConfig{
				Issuer:     "www.example.com",
				HeaderName: "auth_header",
				KeysetURL:  "www.google.com",
			},
			OAuth: &evergreen.OAuthConfig{
				Issuer:      "oauth_issuer",
				ClientID:    "oauth_client_id",
				ConnectorID: "oauth_connector_id",
			},
			BackgroundReauthMinutes: 60,
		},
		AWSInstanceRole: "role",
		Banner:          "banner",
		BannerTheme:     "IMPORTANT",
		Buckets: evergreen.BucketsConfig{
			LogBucket: evergreen.BucketConfig{
				Name: "logs",
				Type: evergreen.BucketTypeS3,
			},
			TestResultsBucket: evergreen.BucketConfig{
				Name: "test_results",
				Type: evergreen.BucketTypeS3,
			},
			Credentials: evergreen.S3Credentials{
				Key:    "aws_key",
				Secret: "aws_secret",
			},
		},
		ConfigDir: "cfg_dir",
		Cost: evergreen.CostConfig{
			FinanceFormula:      0.5,
			SavingsPlanDiscount: 0.3,
			OnDemandDiscount:    0.1,
		},
		ContainerPools: evergreen.ContainerPoolsConfig{
			Pools: []evergreen.ContainerPool{
				{
					Distro:        "valid-distro",
					Id:            "test-pool-1",
					MaxContainers: 100,
					Port:          9999,
				},
			},
		},
		DebugSpawnHosts: evergreen.DebugSpawnHostsConfig{
			SetupScript: "echo 'test setup script'",
		},
		DomainName: "example.com",
		Expansions: map[string]string{"k2": "v2"},
		FWS: evergreen.FWSConfig{
			URL: "fws_url",
		},
		Graphite: evergreen.GraphiteConfig{
			CIOptimizationToken: "graphite_token",
			ServerURL:           "https://graphite.example.com",
		},
		GithubPRCreatorOrg:  "org",
		GithubWebhookSecret: "secret",
		HostInit: evergreen.HostInitConfig{
			HostThrottle:         64,
			ProvisioningThrottle: 100,
			CloudStatusBatchSize: 10,
			MaxTotalDynamicHosts: 500,
		},
		HostJasper: evergreen.HostJasperConfig{
			BinaryName:       "binary",
			DownloadFileName: "download",
			Port:             12345,
			URL:              "url",
			Version:          "version",
		},
		Jira: evergreen.JiraConfig{
			Host:                "host",
			PersonalAccessToken: "personal_access_token",
		},
		TaskLimits: evergreen.TaskLimitsConfig{
			MaxTasksPerVersion: 1000,
		},
		LoggerConfig: evergreen.LoggerConfig{
			Buffer: evergreen.LogBuffering{
				UseAsync:             true,
				Count:                101,
				IncomingBufferFactor: 20,
			},
		},
		LogPath: "logpath",

		Notify: evergreen.NotifyConfig{
			SES: evergreen.SESConfig{
				SenderAddress: "from",
			},
		},
		Overrides: evergreen.OverridesConfig{
			Overrides: []evergreen.Override{
				{SectionID: "section id", Field: "field name", Value: "the value"},
			},
		},
		ParameterStore: evergreen.ParameterStoreConfig{
			Prefix: "/prefix",
		},
		Plugins:   map[string]map[string]any{"k4": {"k5": "v5"}},
		PprofPort: "port",
		ProjectCreation: evergreen.ProjectCreationConfig{
			TotalProjectLimit: 400,
			RepoProjectLimit:  10,
			JiraProject:       "EVG",
			RepoExceptions: []evergreen.OwnerRepo{
				{
					Owner: "owner",
					Repo:  "repo",
				},
			},
		},
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				EC2Keys: []evergreen.EC2Key{
					{
						Name:   "test",
						Key:    "aws_key",
						Secret: "aws_secret",
					},
				},
				DefaultSecurityGroup: "test_security_group",
				MaxVolumeSizePerUser: 200,
				ParserProject: evergreen.ParserProjectS3Config{
					S3Credentials: evergreen.S3Credentials{
						Bucket: "parser_project_bucket",
					},
					Prefix: "parser_project_prefix",
				},
				PersistentDNS: evergreen.PersistentDNSConfig{
					HostedZoneID: "hosted_zone_id",
					Domain:       "example.com",
				},
				AccountRoles: []evergreen.AWSAccountRoleMapping{
					{
						Account: "account",
						Role:    "role",
					},
				},
				IPAMPoolID:         "pool_id",
				ElasticIPUsageRate: 0.3,
			},
			Docker: evergreen.DockerConfig{
				APIVersion: "docker_version",
			},
		},
		RepoTracker: evergreen.RepoTrackerConfig{
			NumNewRepoRevisionsToFetch: 10,
			MaxRepoRevisionsToSearch:   20,
			MaxConcurrentRequests:      30,
		},
		ReleaseMode: evergreen.ReleaseModeConfig{
			DistroMaxHostsFactor:      2.0,
			TargetTimeSecondsOverride: 60,
			IdleTimeSecondsOverride:   120,
		},
		Scheduler: evergreen.SchedulerConfig{
			TaskFinder: "legacy",
		},
		ServiceFlags: evergreen.ServiceFlags{
			TaskDispatchDisabled:            true,
			LargeParserProjectsDisabled:     true,
			HostInitDisabled:                true,
			MonitorDisabled:                 true,
			AlertsDisabled:                  true,
			AgentStartDisabled:              true,
			RepotrackerDisabled:             true,
			SchedulerDisabled:               true,
			CheckBlockedTasksDisabled:       true,
			GithubPRTestingDisabled:         true,
			CLIUpdatesDisabled:              true,
			EventProcessingDisabled:         true,
			JIRANotificationsDisabled:       true,
			SlackNotificationsDisabled:      true,
			EmailNotificationsDisabled:      true,
			WebhookNotificationsDisabled:    true,
			GithubStatusAPIDisabled:         true,
			BackgroundReauthDisabled:        true,
			CloudCleanupDisabled:            true,
			SleepScheduleDisabled:           true,
			StaticAPIKeysDisabled:           true,
			JWTTokenForCLIDisabled:          true,
			SystemFailedTaskRestartDisabled: true,
			CPUDegradedModeDisabled:         true,
			ElasticIPsDisabled:              true,
			ReleaseModeDisabled:             true,
			LegacyUIAdminPageDisabled:       true,
			DebugSpawnHostDisabled:          true,
		},
		SingleTaskDistro: evergreen.SingleTaskDistroConfig{
			ProjectTasksPairs: []evergreen.ProjectTasksPair{
				{
					ProjectID:    "project",
					AllowedTasks: []string{"task0", "task1"},
				},
			},
		},
		SleepSchedule: evergreen.SleepScheduleConfig{
			PermanentlyExemptHosts: []string{"host0", "host1"},
		},
		SSH: evergreen.SSHConfig{
			TaskHostKey: evergreen.SSHKeyPair{
				Name:      "task-host-key",
				SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/top-secret-private-key",
			},
			SpawnHostKey: evergreen.SSHKeyPair{
				Name:      "spawn-host-key",
				SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/confidential-private-key",
			},
		},
		Slack: evergreen.SlackConfig{
			Options: &send.SlackOptions{
				Channel:   "#channel",
				Fields:    true,
				FieldsSet: map[string]bool{},
			},
			Token: "token",
			Level: "info",
			Name:  "name",
		},
		Splunk: evergreen.SplunkConfig{
			SplunkConnectionInfo: send.SplunkConnectionInfo{
				ServerURL: "server",
				Token:     "token",
				Channel:   "channel",
			},
		},
		TestSelection: evergreen.TestSelectionConfig{
			URL: "test_selection_url",
		},
		Triggers: evergreen.TriggerConfig{
			GenerateTaskDistro: "distro",
		},
		Ui: evergreen.UIConfig{
			Url:                "url",
			HttpListenAddr:     "addr",
			Secret:             "secret",
			DefaultProject:     "mci",
			CacheTemplates:     true,
			CsrfKey:            "12345678901234567890123456789012",
			StagingEnvironment: "mine",
		},
		Spawnhost: evergreen.SpawnHostConfig{
			SpawnHostsPerUser:         5,
			UnexpirableHostsPerUser:   2,
			UnexpirableVolumesPerUser: 2,
		},
		Tracer: evergreen.TracerConfig{
			Enabled:                   true,
			CollectorEndpoint:         "www.example.com:443",
			CollectorInternalEndpoint: "svc.cluster.local:4317",
		},
		GitHubCheckRun: evergreen.GitHubCheckRunConfig{
			CheckRunLimit: 0,
		},
		Sage: evergreen.SageConfig{
			BaseURL: "https://sage.example.com",
		},
		ShutdownWaitSeconds: 15,
	}
}

func DisablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = true
}

func EnablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = false
}
