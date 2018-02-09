package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

var testSettings = evergreen.Settings{
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
		TaskFinder:  "task_finder",
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
		CsrfKey:        "csrf",
	},
}

func TestModelConversion(t *testing.T) {
	assert := assert.New(t)
	apiSettings := APIAdminSettings{}

	// test converting from a db model to an API model
	assert.NoError(apiSettings.BuildFromService(&testSettings))
	assert.EqualValues(testSettings.ApiUrl, apiSettings.ApiUrl)
	assert.EqualValues(testSettings.Banner, apiSettings.Banner)
	assert.EqualValues(testSettings.BannerTheme, apiSettings.BannerTheme)
	assert.EqualValues(testSettings.ClientBinariesDir, apiSettings.ClientBinariesDir)
	assert.EqualValues(testSettings.ConfigDir, apiSettings.ConfigDir)
	assert.EqualValues(testSettings.GithubPRCreatorOrg, apiSettings.GithubPRCreatorOrg)
	assert.EqualValues(testSettings.IsNonProd, apiSettings.IsNonProd)
	assert.EqualValues(testSettings.LogPath, apiSettings.LogPath)
	assert.EqualValues(testSettings.PprofPort, apiSettings.PprofPort)
	for k, v := range testSettings.Credentials {
		assert.Contains(apiSettings.Credentials, k)
		assert.EqualValues(v, apiSettings.Credentials[k])
	}
	for k, v := range testSettings.Expansions {
		assert.Contains(apiSettings.Expansions, k)
		assert.EqualValues(v, apiSettings.Expansions[k])
	}
	for k, v := range testSettings.Keys {
		assert.Contains(apiSettings.Keys, k)
		assert.EqualValues(v, apiSettings.Keys[k])
	}
	for k, v := range testSettings.Plugins {
		assert.Contains(apiSettings.Plugins, k)
		for k2, v2 := range v {
			assert.Contains(apiSettings.Plugins[k], k2)
			assert.EqualValues(v2, apiSettings.Plugins[k][k2])
		}
	}

	assert.EqualValues(testSettings.Alerts.SMTP.From, apiSettings.Alerts.SMTP.From)
	assert.EqualValues(testSettings.Alerts.SMTP.Port, apiSettings.Alerts.SMTP.Port)
	assert.Equal(len(testSettings.Alerts.SMTP.AdminEmail), len(apiSettings.Alerts.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Amboy.Name, apiSettings.Amboy.Name)
	assert.EqualValues(testSettings.Amboy.LocalStorage, apiSettings.Amboy.LocalStorage)
	assert.EqualValues(testSettings.Api.HttpListenAddr, apiSettings.Api.HttpListenAddr)
	assert.EqualValues(testSettings.AuthConfig.Crowd.Username, apiSettings.AuthConfig.Crowd.Username)
	assert.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, apiSettings.AuthConfig.Naive.Users[0].Username)
	assert.EqualValues(testSettings.AuthConfig.Github.ClientId, apiSettings.AuthConfig.Github.ClientId)
	assert.Equal(len(testSettings.AuthConfig.Github.Users), len(apiSettings.AuthConfig.Github.Users))
	assert.EqualValues(testSettings.HostInit.SSHTimeoutSeconds, apiSettings.HostInit.SSHTimeoutSeconds)
	assert.EqualValues(testSettings.Jira.Username, apiSettings.Jira.Username)
	assert.EqualValues(testSettings.LoggerConfig.DefaultLevel, apiSettings.LoggerConfig.DefaultLevel)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.Count, apiSettings.LoggerConfig.Buffer.Count)
	assert.EqualValues(testSettings.NewRelic.ApplicationName, apiSettings.NewRelic.ApplicationName)
	assert.EqualValues(testSettings.Notify.SMTP.From, apiSettings.Notify.SMTP.From)
	assert.EqualValues(testSettings.Notify.SMTP.Port, apiSettings.Notify.SMTP.Port)
	assert.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(apiSettings.Notify.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Providers.AWS.Id, apiSettings.Providers.AWS.Id)
	assert.EqualValues(testSettings.Providers.Docker.APIVersion, apiSettings.Providers.Docker.APIVersion)
	assert.EqualValues(testSettings.Providers.GCE.PrivateKey, apiSettings.Providers.GCE.PrivateKey)
	assert.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, apiSettings.Providers.OpenStack.IdentityEndpoint)
	assert.EqualValues(testSettings.Providers.VSphere.Host, apiSettings.Providers.VSphere.Host)
	assert.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, apiSettings.RepoTracker.MaxConcurrentRequests)
	assert.EqualValues(testSettings.Scheduler.TaskFinder, apiSettings.Scheduler.TaskFinder)
	assert.EqualValues(testSettings.ServiceFlags.HostinitDisabled, apiSettings.ServiceFlags.HostinitDisabled)
	assert.EqualValues(testSettings.Slack.Level, apiSettings.Slack.Level)
	assert.EqualValues(testSettings.Slack.Options.Channel, apiSettings.Slack.Options.Channel)
	assert.EqualValues(testSettings.Splunk.Channel, apiSettings.Splunk.Channel)
	assert.EqualValues(testSettings.Ui.HttpListenAddr, apiSettings.Ui.HttpListenAddr)

	// test converting from the API model back to a DB model
	dbSettings, err := apiSettings.ToService()
	assert.NoError(err)
	assert.Equal(testSettings, dbSettings.(evergreen.Settings))
}

func TestRestart(t *testing.T) {
	assert := assert.New(t)
	restartResp := &RestartTasksResponse{
		TasksRestarted: []string{"task1", "task2", "task3"},
		TasksErrored:   []string{"task4", "task5"},
	}

	apiResp := RestartTasksResponse{}
	assert.NoError(apiResp.BuildFromService(restartResp))
	assert.Equal(3, len(apiResp.TasksRestarted))
	assert.Equal(2, len(apiResp.TasksErrored))
}
