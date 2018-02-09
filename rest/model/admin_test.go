package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestModelConversion(t *testing.T) {
	assert := assert.New(t)
	testSettings := testutil.MockConfig()
	apiSettings := APIAdminSettings{}

	// test converting from a db model to an API model
	assert.NoError(apiSettings.BuildFromService(testSettings))
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
	assert.Equal(*testSettings, dbSettings.(evergreen.Settings))
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
