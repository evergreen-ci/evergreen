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
	TestDir                    = "config_test"
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
			Okta: &evergreen.OktaConfig{
				ClientID:           "id",
				ClientSecret:       "secret",
				Issuer:             "issuer",
				UserGroup:          "group",
				ExpireAfterMinutes: 60,
			},
			Naive: &evergreen.NaiveAuthConfig{
				Users: []evergreen.AuthUser{evergreen.AuthUser{Username: "user", Password: "pw"}},
			},
			OnlyAPI: &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{evergreen.OnlyAPIUser{Username: "api_user", Key: "key", Roles: []string{"admin"}}},
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
			HostThrottle:      64,
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
		NewRelic: evergreen.NewRelicConfig{
			AccountID:     "123123123",
			TrustKey:      "098765",
			AgentID:       "45678",
			LicenseKey:    "890765",
			ApplicationID: "8888888",
		},
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
			BackgroundReauthDisabled:     true,
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
		},
		Splunk: send.SplunkConnectionInfo{
			ServerURL: "server",
			Token:     "token",
			Channel:   "channel",
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
		SpawnHostsPerUser:       5,
		UnexpirableHostsPerUser: 2,
	}
}

func DisablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = true
}

func EnablePermissionsForTests() {
	evergreen.PermissionSystemDisabled = false
}
