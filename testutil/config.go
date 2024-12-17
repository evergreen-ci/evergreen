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
			Credentials: evergreen.S3Credentials{
				Key:    "aws_key",
				Secret: "aws_secret",
			},
		},
		Cedar: evergreen.CedarConfig{
			BaseURL: "url.com",
			RPCPort: "7070",
			User:    "cedar-user",
			APIKey:  "cedar-key",
		},
		CommitQueue: evergreen.CommitQueueConfig{
			MergeTaskDistro: "distro",
			CommitterName:   "Evergreen Commit Queue",
			CommitterEmail:  "evergreen@mongodb.com",
		},
		ConfigDir: "cfg_dir",
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
		DomainName:          "example.com",
		Expansions:          map[string]string{"k2": "v2"},
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
			Host: "host",
			BasicAuthConfig: evergreen.JiraBasicAuthConfig{
				Username: "username",
				Password: "password",
			},
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
		NewRelic: evergreen.NewRelicConfig{
			AccountID:     "123123123",
			TrustKey:      "098765",
			AgentID:       "45678",
			LicenseKey:    "890765",
			ApplicationID: "8888888",
		},
		Notify: evergreen.NotifyConfig{
			SES: evergreen.SESConfig{
				SenderAddress: "from",
			},
		},
		ParameterStore: evergreen.ParameterStoreConfig{
			Prefix: "/prefix",
		},
		Plugins: map[string]map[string]interface{}{"k4": {"k5": "v5"}},
		PodLifecycle: evergreen.PodLifecycleConfig{
			MaxParallelPodRequests:      2000,
			MaxPodDefinitionCleanupRate: 100,
			MaxSecretCleanupRate:        200,
		},
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
				TaskSync: evergreen.S3Credentials{
					Key:    "task_sync_key",
					Secret: "task_sync_secret",
					Bucket: "task_sync_bucket",
				},
				TaskSyncRead: evergreen.S3Credentials{
					Key:    "task_sync_read_key",
					Secret: "task_sync_read_secret",
					Bucket: "task_sync_bucket",
				},
				Pod: evergreen.AWSPodConfig{
					Role:   "role",
					Region: "region",
					ECS: evergreen.ECSConfig{
						MaxCPU:               2048,
						MaxMemoryMB:          4096,
						TaskDefinitionPrefix: "ecs_prefix",
						TaskRole:             "task_role",
						ExecutionRole:        "execution_role",
						LogRegion:            "log_region",
						LogStreamPrefix:      "log_stream_prefix",
						LogGroup:             "log_group",
						AWSVPC: evergreen.AWSVPCConfig{
							Subnets:        []string{"subnet-12345"},
							SecurityGroups: []string{"sg-12345"},
						},
						Clusters: []evergreen.ECSClusterConfig{
							{
								Name: "linux_cluster_name",
								OS:   evergreen.ECSOSLinux,
							},
							{
								Name: "windows_cluster_name",
								OS:   evergreen.ECSOSLinux,
							},
						},
						CapacityProviders: []evergreen.ECSCapacityProvider{
							{
								Name: "linux_capacity_provider_name",
								OS:   evergreen.ECSOSLinux,
								Arch: evergreen.ECSArchAMD64,
							},
							{
								Name:           "windows_capacity_provider_name",
								OS:             evergreen.ECSOSWindows,
								Arch:           evergreen.ECSArchAMD64,
								WindowsVersion: evergreen.ECSWindowsServer2022,
							},
						},
					},
					SecretsManager: evergreen.SecretsManagerConfig{
						SecretPrefix: "secret_prefix",
					},
				},
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
		Scheduler: evergreen.SchedulerConfig{
			TaskFinder: "legacy",
		},
		ServiceFlags: evergreen.ServiceFlags{
			TaskDispatchDisabled:            true,
			LargeParserProjectsDisabled:     true,
			HostInitDisabled:                true,
			PodInitDisabled:                 true,
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
			PodAllocatorDisabled:            true,
			UnrecognizedPodCleanupDisabled:  true,
			CloudCleanupDisabled:            true,
			SleepScheduleDisabled:           true,
			SystemFailedTaskRestartDisabled: true,
			CPUDegradedModeDisabled:         true,
			ParameterStoreDisabled:          true,
		},
		SleepSchedule: evergreen.SleepScheduleConfig{
			PermanentlyExemptHosts: []string{"host0", "host1"},
		},
		SSHKeyDirectory: "/ssh_key_directory",
		SSHKeyPairs: []evergreen.SSHKeyPair{
			{
				Name:    "key",
				Public:  "public",
				Private: "private",
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
			Url:            "url",
			HelpUrl:        "helpurl",
			HttpListenAddr: "addr",
			Secret:         "secret",
			DefaultProject: "mci",
			CacheTemplates: true,
			CsrfKey:        "12345678901234567890123456789012",
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
		ShutdownWaitSeconds: 15,
	}
}

func DisablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = true
}

func EnablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = false
}
