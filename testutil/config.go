package testutil

import (
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/send"
)

const (
	TestDir      = "config_test"
	TestSettings = "evg_settings.yml"
)

// TestConfig creates test settings from a test config.
func TestConfig() *evergreen.Settings {
	file := filepath.Join(evergreen.FindEvergreenHome(), TestDir, TestSettings)
	settings, err := evergreen.NewSettings(file)
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
			SMTP: &evergreen.SMTPConfig{
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
			Name:           "amboy",
			DB:             "db",
			PoolSizeLocal:  10,
			PoolSizeRemote: 20,
			LocalStorage:   30,
		},
		Api: evergreen.APIConfig{
			HttpListenAddr:      "addr",
			GithubWebhookSecret: "secret",
		},
		ApiUrl: "api",
		AuthConfig: evergreen.AuthConfig{
			Crowd: &evergreen.CrowdConfig{
				Username: "crowduser",
				Password: "crowdpw",
				Urlroot:  "crowdurl",
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
		Banner:             "banner",
		BannerTheme:        "important",
		ClientBinariesDir:  "bin_dir",
		ConfigDir:          "cfg_dir",
		Credentials:        map[string]string{"k1": "v1"},
		Expansions:         map[string]string{"k2": "v2"},
		GithubPRCreatorOrg: "org",
		HostInit: evergreen.HostInitConfig{
			SSHTimeoutSeconds: 10,
		},
		IsNonProd: true,
		Jira: evergreen.JiraConfig{
			Host:           "host",
			Username:       "username",
			Password:       "password",
			DefaultProject: "proj",
		},
		Keys:    map[string]string{"k3": "v3"},
		LogPath: "logpath",
		NewRelic: evergreen.NewRelicConfig{
			ApplicationName: "new_relic",
			LicenseKey:      "key",
		},
		Notify: evergreen.NotifyConfig{
			SMTP: &evergreen.SMTPConfig{
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
				Secret: "aws_secret",
				Id:     "aws",
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
			MergeToggle: 10,
			TaskFinder:  "legacy",
		},
		ServiceFlags: evergreen.ServiceFlags{
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
		},
		Slack: evergreen.SlackConfig{
			Options: &send.SlackOptions{
				Channel:   "channel",
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
		Ui: evergreen.UIConfig{
			Url:            "url",
			HelpUrl:        "helpurl",
			HttpListenAddr: "addr",
			Secret:         "secret",
			DefaultProject: "mci",
			CacheTemplates: true,
			SecureCookies:  true,
			CsrfKey:        "12345678901234567890123456789012",
		},
	}
}
