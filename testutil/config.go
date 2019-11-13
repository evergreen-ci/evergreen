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

const (
	TestDir                    = "config_test"
	TestSettings               = "evg_settings.yml"
	testSettingsWithAuthTokens = "evg_settings_with_3rd_party_defaults.yml"
)

func init() {
	if flag.Lookup("test.v") == nil {
		grip.Alert("called init() in testutil for production code.")
	} else if evergreen.GetEnvironment() == nil {
		ctx := context.Background()

		path := filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings)
		env, err := evergreen.NewEnvironment(ctx, path, nil)
		grip.EmergencyPanic(message.WrapError(err, message.Fields{
			"note": "could not initialize test environment",
			"path": filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings),
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
		Alerts: evergreen.AlertsConfig{
			SMTP: evergreen.SMTPConfig{
				Server:     "server",
				Port:       2285,
				UseSSL:     true,
				Username:   "username",
				Password:   "password",
				From:       "from",
				AdminEmail: []string{"email"},
			},
		},
		Amboy: evergreen.AmboyConfig{
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
			Naive: &evergreen.NaiveAuthConfig{
				Users: []*evergreen.AuthUser{&evergreen.AuthUser{Username: "user", Password: "pw"}},
			},
			Github: &evergreen.GithubAuthConfig{
				ClientId:     "ghclient",
				ClientSecret: "ghsecret",
				Users:        []string{"ghuser"},
				Organization: "ghorg",
			},
		},
		Banner:            "banner",
		BannerTheme:       "important",
		ClientBinariesDir: "bin_dir",
		CommitQueue: evergreen.CommitQueueConfig{
			MergeTaskDistro: "distro",
			CommitterName:   "Evergreen Commit Queue",
			CommitterEmail:  "evergreen@mongodb.com",
		},
		ConfigDir: "cfg_dir",
		ContainerPools: evergreen.ContainerPoolsConfig{
			Pools: []evergreen.ContainerPool{
				evergreen.ContainerPool{
					Distro:        "valid-distro",
					Id:            "test-pool-1",
					MaxContainers: 100,
					Port:          9999,
				},
			},
		},
		Credentials:        map[string]string{"k1": "v1"},
		DomainName:         "example.com",
		Expansions:         map[string]string{"k2": "v2"},
		Bugsnag:            "u-12345",
		GithubPRCreatorOrg: "org",
		HostInit: evergreen.HostInitConfig{
			SSHTimeoutSeconds: 10,
		},
		HostJasper: evergreen.HostJasperConfig{
			BinaryName:       "binary",
			DownloadFileName: "download",
			Port:             12345,
			URL:              "url",
			Version:          "version",
		},
		Jira: evergreen.JiraConfig{
			Host:           "host",
			Username:       "username",
			Password:       "password",
			DefaultProject: "proj",
		},
		Keys:    map[string]string{"k3": "v3"},
		LogPath: "logpath",
		Notify: evergreen.NotifyConfig{
			SMTP: evergreen.SMTPConfig{
				Server:     "server",
				Port:       2285,
				UseSSL:     true,
				Username:   "username",
				Password:   "password",
				From:       "from",
				AdminEmail: []string{"email"},
			},
		},
		Plugins:   map[string]map[string]interface{}{"k4": map[string]interface{}{"k5": "v5"}},
		PprofPort: "port",
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				EC2Keys: []evergreen.EC2Key{
					{
						Name:   "test",
						Region: "us-east-1",
						Key:    "aws_key",
						Secret: "aws_secret",
					},
				},
				DefaultSecurityGroup: "test_security_group",

				// Legacy
				EC2Secret:            "aws_secret",
				EC2Key:               "aws",
				MaxVolumeSizePerUser: 200,
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
			TaskDispatchDisabled:         true,
			HostInitDisabled:             true,
			MonitorDisabled:              true,
			AlertsDisabled:               true,
			AgentStartDisabled:           true,
			RepotrackerDisabled:          true,
			SchedulerDisabled:            true,
			GithubPRTestingDisabled:      true,
			CLIUpdatesDisabled:           true,
			EventProcessingDisabled:      true,
			JIRANotificationsDisabled:    true,
			SlackNotificationsDisabled:   true,
			EmailNotificationsDisabled:   true,
			WebhookNotificationsDisabled: true,
			GithubStatusAPIDisabled:      true,
		},
		Slack: evergreen.SlackConfig{
			Options: &send.SlackOptions{
				Channel:   "#channel",
				Fields:    true,
				FieldsSet: map[string]bool{},
			},
			Token: "token",
			Level: "info",
		},
		Splunk: send.SplunkConnectionInfo{
			ServerURL: "server",
			Token:     "token",
			Channel:   "channel",
		},
		SuperUsers: []string{"user"},
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
		SpawnHostsPerUser:       5,
		UnexpirableHostsPerUser: 2,
	}
}
