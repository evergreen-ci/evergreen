package testutil

import (
	"context"
	"flag"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
)

var ExecutionEnvironmentType = "production"

const (
	TestDir = "config_test"
	// TestSettings contains the default admin settings suitable for testing
	// that depends on the global environment.
	TestSettings               = "evg_settings.yml"
	testSettingsWithAuthTokens = "evg_settings_with_3rd_party_defaults.yml"
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
		env, err := evergreen.NewEnvironment(ctx, path, nil)

		grip.EmergencyPanic(message.WrapError(err, message.Fields{
			"message": "could not initialize test environment",
			"path":    filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings),
		}))

		evergreen.SetEnvironment(env)
	}
}

func NewEnvironment(ctx context.Context, t *testing.T) evergreen.Environment {
	env, err := evergreen.NewEnvironment(ctx, filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings), nil)
	require.NoError(t, err)
	return env
}

// TestConfig creates test settings from a test config.
func TestConfig() *evergreen.Settings {
	return loadConfig(TestDir, TestSettings)
}

func TestConfigWithDefaultAuthTokens() *evergreen.Settings {
	return loadConfig(TestDir, testSettingsWithAuthTokens)
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
			Name:       "amboy",
			SingleName: "single",
			DBConnection: evergreen.AmboyDBConfig{
				Database: "db",
				URL:      "mongodb://localhost:27017",
				Username: "user",
				Password: "password",
			},
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
		Api: evergreen.APIConfig{
			HttpListenAddr:      "addr",
			GithubWebhookSecret: "secret",
		},
		ApiUrl: "api",
		AuthConfig: evergreen.AuthConfig{
			LDAP: &evergreen.LDAPConfig{
				URL:                "url",
				Port:               "port",
				UserPath:           "path",
				ServicePath:        "bot",
				Group:              "group",
				ExpireAfterMinutes: "60",
			},
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
				ReadWrite: []string{evergreen.AuthLDAPKey},
				ReadOnly:  []string{evergreen.AuthNaiveKey},
			},
			PreferredType:           evergreen.AuthLDAPKey,
			BackgroundReauthMinutes: 60,
		},
		AWSInstanceRole: "role",
		Banner:          "banner",
		BannerTheme:     "important",
		Buckets: evergreen.BucketsConfig{
			LogBucket: evergreen.BucketConfig{
				Name: "logs",
				Type: evergreen.BucketTypeS3,
			},
		},
		Cedar: evergreen.CedarConfig{
			BaseURL: "url.com",
			RPCPort: "7070",
			User:    "cedar-user",
			APIKey:  "cedar-key",
		},
		ClientBinariesDir: "bin_dir",
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
		Credentials: map[string]string{"k1": "v1"},
		DataPipes: evergreen.DataPipesConfig{
			Host:         "url",
			Region:       "us-east-1",
			AWSAccessKey: "access",
			AWSSecretKey: "secret",
			AWSToken:     "token",
		},
		DomainName:         "example.com",
		Expansions:         map[string]string{"k2": "v2"},
		GithubPRCreatorOrg: "org",
		HostInit: evergreen.HostInitConfig{
			HostThrottle:         64,
			ProvisioningThrottle: 100,
			CloudStatusBatchSize: 10,
			MaxTotalDynamicHosts: 500,
			S3BaseURL:            "s3_base_url",
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
			DefaultProject: "proj",
		},
		Keys: map[string]string{"k3": "v3"},
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
		Plugins: map[string]map[string]interface{}{"k4": {"k5": "v5"}},
		PodLifecycle: evergreen.PodLifecycleConfig{
			S3BaseURL:                   "s3_base_url",
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
						Key:    "parser_project_key",
						Secret: "parser_project_secret",
						Bucket: "parser_project_bucket",
					},
					Prefix: "parser_project_prefix",
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
			GCE: evergreen.GCEConfig{
				ClientEmail:  "gce_email",
				PrivateKey:   "gce_key",
				PrivateKeyID: "gce_key_id",
				TokenURI:     "gce_token",
			},
			OpenStack: evergreen.OpenStackConfig{
				IdentityEndpoint: "endpoint",
				Username:         "username",
				Password:         "password",
				DomainName:       "domain",
				ProjectName:      "project",
				ProjectID:        "project_id",
				Region:           "region",
			},
			VSphere: evergreen.VSphereConfig{
				Host:     "host",
				Username: "vsphere",
				Password: "vsphere_pass",
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
			TaskDispatchDisabled:           true,
			HostInitDisabled:               true,
			PodInitDisabled:                true,
			S3BinaryDownloadsDisabled:      true,
			MonitorDisabled:                true,
			AlertsDisabled:                 true,
			AgentStartDisabled:             true,
			RepotrackerDisabled:            true,
			SchedulerDisabled:              true,
			CheckBlockedTasksDisabled:      true,
			GithubPRTestingDisabled:        true,
			CLIUpdatesDisabled:             true,
			EventProcessingDisabled:        true,
			JIRANotificationsDisabled:      true,
			SlackNotificationsDisabled:     true,
			EmailNotificationsDisabled:     true,
			WebhookNotificationsDisabled:   true,
			GithubStatusAPIDisabled:        true,
			BackgroundReauthDisabled:       true,
			PodAllocatorDisabled:           true,
			UnrecognizedPodCleanupDisabled: true,
			CloudCleanupDisabled:           true,
			LegacyUIPublicAccessDisabled:   true,
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
			Enabled:           true,
			CollectorEndpoint: "localhost:4317",
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
