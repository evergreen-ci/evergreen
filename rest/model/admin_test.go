package model

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigModelHasMatchingFieldNames(t *testing.T) {
	assert := assert.New(t)
	config := evergreen.Settings{}

	matched := map[string]bool{}

	configRef := reflect.TypeOf(&config).Elem()
	for i := 0; i < configRef.NumField(); i++ {
		configFieldName := configRef.Field(i).Name
		matched[configFieldName] = true
	}

	apiConfig := APIAdminSettings{}
	apiConfigRef := reflect.TypeOf(&apiConfig).Elem()
	for i := 0; i < apiConfigRef.NumField(); i++ {
		configFieldName := apiConfigRef.Field(i).Name
		v, ok := matched[configFieldName]
		assert.True(v)
		assert.True(ok, fmt.Sprintf("%s is missing from evergreen.Settings", configFieldName))
		if ok {
			matched[configFieldName] = false
		}
	}

	exclude := []string{"Id", "CredentialsNew", "Database", "KeysNew", "ExpansionsNew", "PluginsNew"}
	for k, v := range matched {
		if !utility.StringSliceContains(exclude, k) {
			assert.False(v, fmt.Sprintf("%s is missing from APIAdminSettings", k))
		}
	}
}

func TestModelConversion(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	testSettings := testutil.MockConfig()
	apiSettings := NewConfigModel()

	// test converting from a db model to an API model
	assert.NoError(apiSettings.BuildFromService(testSettings))
	assert.Equal(testSettings.ApiUrl, *apiSettings.ApiUrl)
	assert.Equal(testSettings.Banner, *apiSettings.Banner)
	assert.EqualValues(testSettings.BannerTheme, *apiSettings.BannerTheme)
	assert.Equal(testSettings.ClientBinariesDir, *apiSettings.ClientBinariesDir)
	assert.Equal(testSettings.ConfigDir, *apiSettings.ConfigDir)
	assert.Equal(testSettings.GithubPRCreatorOrg, *apiSettings.GithubPRCreatorOrg)
	assert.Equal(testSettings.LogPath, *apiSettings.LogPath)
	assert.Equal(testSettings.PprofPort, *apiSettings.PprofPort)
	for k, v := range testSettings.Credentials {
		assert.Contains(apiSettings.Credentials, k)
		assert.Equal(v, apiSettings.Credentials[k])
	}
	for k, v := range testSettings.Expansions {
		assert.Contains(apiSettings.Expansions, k)
		assert.Equal(v, apiSettings.Expansions[k])
	}
	for k, v := range testSettings.Keys {
		assert.Contains(apiSettings.Keys, k)
		assert.Equal(v, apiSettings.Keys[k])
	}
	for k, v := range testSettings.Plugins {
		assert.Contains(apiSettings.Plugins, k)
		for k2, v2 := range v {
			assert.Contains(apiSettings.Plugins[k], k2)
			assert.Equal(v2, apiSettings.Plugins[k][k2])
		}
	}
	require.Len(apiSettings.SSHKeyPairs, len(testSettings.SSHKeyPairs))
	for i := 0; i < len(testSettings.SSHKeyPairs); i++ {
		assert.Equal(testSettings.SSHKeyPairs[i].Name, utility.FromStringPtr(apiSettings.SSHKeyPairs[i].Name))
		assert.Equal(testSettings.SSHKeyPairs[i].Public, utility.FromStringPtr(apiSettings.SSHKeyPairs[i].Public))
		assert.Equal(testSettings.SSHKeyPairs[i].Private, utility.FromStringPtr(apiSettings.SSHKeyPairs[i].Private))
	}
	assert.Equal(testSettings.ShutdownWaitSeconds, *apiSettings.ShutdownWaitSeconds)

	assert.EqualValues(testSettings.Alerts.SMTP.From, utility.FromStringPtr(apiSettings.Alerts.SMTP.From))
	assert.EqualValues(testSettings.Alerts.SMTP.Port, apiSettings.Alerts.SMTP.Port)
	assert.Equal(len(testSettings.Alerts.SMTP.AdminEmail), len(apiSettings.Alerts.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Amboy.Name, utility.FromStringPtr(apiSettings.Amboy.Name))
	assert.EqualValues(testSettings.Amboy.LocalStorage, apiSettings.Amboy.LocalStorage)
	assert.EqualValues(testSettings.Amboy.GroupDefaultWorkers, apiSettings.Amboy.GroupDefaultWorkers)
	assert.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, apiSettings.Amboy.GroupBackgroundCreateFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, apiSettings.Amboy.GroupPruneFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupTTLMinutes, apiSettings.Amboy.GroupTTLMinutes)
	assert.EqualValues(testSettings.Amboy.LockTimeoutMinutes, apiSettings.Amboy.LockTimeoutMinutes)
	assert.EqualValues(testSettings.Amboy.SampleSize, apiSettings.Amboy.SampleSize)
	assert.EqualValues(testSettings.Amboy.RequireRemotePriority, apiSettings.Amboy.RequireRemotePriority)
	assert.EqualValues(testSettings.Amboy.Retry, apiSettings.Amboy.Retry)
	assert.EqualValues(testSettings.Api.HttpListenAddr, utility.FromStringPtr(apiSettings.Api.HttpListenAddr))
	assert.EqualValues(testSettings.AuthConfig.PreferredType, utility.FromStringPtr(apiSettings.AuthConfig.PreferredType))
	assert.EqualValues(testSettings.AuthConfig.LDAP.URL, utility.FromStringPtr(apiSettings.AuthConfig.LDAP.URL))
	assert.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, utility.FromStringPtr(apiSettings.AuthConfig.Naive.Users[0].Username))
	assert.EqualValues(testSettings.AuthConfig.Okta.ClientID, utility.FromStringPtr(apiSettings.AuthConfig.Okta.ClientID))
	assert.EqualValues(testSettings.AuthConfig.Github.ClientId, utility.FromStringPtr(apiSettings.AuthConfig.Github.ClientId))
	assert.EqualValues(testSettings.AuthConfig.Multi.ReadWrite[0], apiSettings.AuthConfig.Multi.ReadWrite[0])
	assert.Equal(len(testSettings.AuthConfig.Github.Users), len(apiSettings.AuthConfig.Github.Users))
	assert.EqualValues(testSettings.CommitQueue.MergeTaskDistro, utility.FromStringPtr(apiSettings.CommitQueue.MergeTaskDistro))
	assert.EqualValues(testSettings.CommitQueue.CommitterName, utility.FromStringPtr(apiSettings.CommitQueue.CommitterName))
	assert.EqualValues(testSettings.CommitQueue.CommitterEmail, utility.FromStringPtr(apiSettings.CommitQueue.CommitterEmail))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Distro, utility.FromStringPtr(apiSettings.ContainerPools.Pools[0].Distro))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Id, utility.FromStringPtr(apiSettings.ContainerPools.Pools[0].Id))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].MaxContainers, apiSettings.ContainerPools.Pools[0].MaxContainers)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Port, apiSettings.ContainerPools.Pools[0].Port)
	assert.Equal(testSettings.HostJasper.BinaryName, utility.FromStringPtr(apiSettings.HostJasper.BinaryName))
	assert.Equal(testSettings.HostJasper.DownloadFileName, utility.FromStringPtr(apiSettings.HostJasper.DownloadFileName))
	assert.Equal(testSettings.HostJasper.Port, apiSettings.HostJasper.Port)
	assert.Equal(testSettings.HostJasper.URL, utility.FromStringPtr(apiSettings.HostJasper.URL))
	assert.Equal(testSettings.HostJasper.Version, utility.FromStringPtr(apiSettings.HostJasper.Version))
	assert.EqualValues(testSettings.HostInit.HostThrottle, apiSettings.HostInit.HostThrottle)
	assert.EqualValues(testSettings.HostInit.ProvisioningThrottle, apiSettings.HostInit.ProvisioningThrottle)
	assert.EqualValues(testSettings.HostInit.CloudStatusBatchSize, apiSettings.HostInit.CloudStatusBatchSize)
	assert.EqualValues(testSettings.HostInit.MaxTotalDynamicHosts, apiSettings.HostInit.MaxTotalDynamicHosts)
	assert.EqualValues(testSettings.HostInit.S3BaseURL, utility.FromStringPtr(apiSettings.HostInit.S3BaseURL))
	assert.EqualValues(testSettings.Jira.BasicAuthConfig.Username, utility.FromStringPtr(apiSettings.Jira.BasicAuthConfig.Username))
	assert.EqualValues(testSettings.LoggerConfig.DefaultLevel, utility.FromStringPtr(apiSettings.LoggerConfig.DefaultLevel))
	assert.EqualValues(testSettings.LoggerConfig.Buffer.Count, apiSettings.LoggerConfig.Buffer.Count)
	assert.EqualValues(testSettings.Notify.SMTP.From, utility.FromStringPtr(apiSettings.Notify.SMTP.From))
	assert.EqualValues(testSettings.Notify.SMTP.Port, apiSettings.Notify.SMTP.Port)
	assert.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(apiSettings.Notify.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Name, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Name))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Key, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Key))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Secret, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Secret))
	assert.EqualValues(testSettings.Providers.AWS.DefaultSecurityGroup, utility.FromStringPtr(apiSettings.Providers.AWS.DefaultSecurityGroup))
	assert.EqualValues(testSettings.Providers.AWS.MaxVolumeSizePerUser, *apiSettings.Providers.AWS.MaxVolumeSizePerUser)
	assert.EqualValues(testSettings.Providers.AWS.S3.Key, utility.FromStringPtr(apiSettings.Providers.AWS.S3.Key))
	assert.EqualValues(testSettings.Providers.AWS.S3.Secret, utility.FromStringPtr(apiSettings.Providers.AWS.S3.Secret))
	assert.EqualValues(testSettings.Providers.AWS.S3.Bucket, utility.FromStringPtr(apiSettings.Providers.AWS.S3.Bucket))
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Key, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSync.Key))
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Secret, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSync.Secret))
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Bucket, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSync.Bucket))
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Key, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSyncRead.Key))
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Secret, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSyncRead.Secret))
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Bucket, utility.FromStringPtr(apiSettings.Providers.AWS.TaskSync.Bucket))
	assert.EqualValues(testSettings.Providers.Docker.APIVersion, utility.FromStringPtr(apiSettings.Providers.Docker.APIVersion))
	assert.EqualValues(testSettings.Providers.GCE.ClientEmail, utility.FromStringPtr(apiSettings.Providers.GCE.ClientEmail))
	assert.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, utility.FromStringPtr(apiSettings.Providers.OpenStack.IdentityEndpoint))
	assert.EqualValues(testSettings.Providers.VSphere.Host, utility.FromStringPtr(apiSettings.Providers.VSphere.Host))
	assert.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, apiSettings.RepoTracker.MaxConcurrentRequests)
	assert.EqualValues(testSettings.Scheduler.TaskFinder, utility.FromStringPtr(apiSettings.Scheduler.TaskFinder))
	assert.EqualValues(testSettings.ServiceFlags.HostInitDisabled, apiSettings.ServiceFlags.HostInitDisabled)
	assert.EqualValues(testSettings.ServiceFlags.S3BinaryDownloadsDisabled, apiSettings.ServiceFlags.S3BinaryDownloadsDisabled)
	assert.EqualValues(testSettings.Slack.Level, utility.FromStringPtr(apiSettings.Slack.Level))
	assert.EqualValues(testSettings.Slack.Options.Channel, utility.FromStringPtr(apiSettings.Slack.Options.Channel))
	assert.EqualValues(testSettings.Splunk.Channel, utility.FromStringPtr(apiSettings.Splunk.Channel))
	assert.EqualValues(testSettings.Triggers.GenerateTaskDistro, utility.FromStringPtr(apiSettings.Triggers.GenerateTaskDistro))
	assert.EqualValues(testSettings.Ui.HttpListenAddr, utility.FromStringPtr(apiSettings.Ui.HttpListenAddr))
	assert.Equal(testSettings.Spawnhost.SpawnHostsPerUser, *apiSettings.Spawnhost.SpawnHostsPerUser)
	assert.Equal(testSettings.Spawnhost.UnexpirableHostsPerUser, *apiSettings.Spawnhost.UnexpirableHostsPerUser)
	assert.Equal(testSettings.Spawnhost.UnexpirableVolumesPerUser, *apiSettings.Spawnhost.UnexpirableVolumesPerUser)

	// test converting from the API model back to a DB model
	dbInterface, err := apiSettings.ToService()
	assert.NoError(err)
	dbSettings := dbInterface.(evergreen.Settings)
	assert.EqualValues(testSettings.Alerts.SMTP.From, dbSettings.Alerts.SMTP.From)
	assert.EqualValues(testSettings.Alerts.SMTP.Port, dbSettings.Alerts.SMTP.Port)
	assert.Equal(len(testSettings.Alerts.SMTP.AdminEmail), len(dbSettings.Alerts.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Amboy.Name, dbSettings.Amboy.Name)
	assert.EqualValues(testSettings.Amboy.LocalStorage, dbSettings.Amboy.LocalStorage)
	assert.EqualValues(testSettings.Amboy.GroupDefaultWorkers, dbSettings.Amboy.GroupDefaultWorkers)
	assert.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, dbSettings.Amboy.GroupBackgroundCreateFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, dbSettings.Amboy.GroupPruneFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.RequireRemotePriority, dbSettings.Amboy.RequireRemotePriority)
	assert.EqualValues(testSettings.Amboy.GroupTTLMinutes, dbSettings.Amboy.GroupTTLMinutes)
	assert.EqualValues(testSettings.Amboy.LockTimeoutMinutes, dbSettings.Amboy.LockTimeoutMinutes)
	assert.EqualValues(testSettings.Amboy.SampleSize, dbSettings.Amboy.SampleSize)
	assert.EqualValues(testSettings.Api.HttpListenAddr, dbSettings.Api.HttpListenAddr)
	assert.EqualValues(testSettings.AuthConfig.LDAP.URL, dbSettings.AuthConfig.LDAP.URL)
	assert.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, dbSettings.AuthConfig.Naive.Users[0].Username)
	assert.EqualValues(testSettings.AuthConfig.Github.ClientId, dbSettings.AuthConfig.Github.ClientId)
	assert.Equal(len(testSettings.AuthConfig.Github.Users), len(dbSettings.AuthConfig.Github.Users))
	assert.EqualValues(testSettings.AuthConfig.Multi.ReadWrite[0], dbSettings.AuthConfig.Multi.ReadWrite[0])
	assert.EqualValues(testSettings.CommitQueue.MergeTaskDistro, dbSettings.CommitQueue.MergeTaskDistro)
	assert.EqualValues(testSettings.CommitQueue.CommitterName, dbSettings.CommitQueue.CommitterName)
	assert.EqualValues(testSettings.CommitQueue.CommitterEmail, dbSettings.CommitQueue.CommitterEmail)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Distro, dbSettings.ContainerPools.Pools[0].Distro)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Id, dbSettings.ContainerPools.Pools[0].Id)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].MaxContainers, dbSettings.ContainerPools.Pools[0].MaxContainers)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Port, dbSettings.ContainerPools.Pools[0].Port)
	assert.EqualValues(testSettings.HostInit.HostThrottle, dbSettings.HostInit.HostThrottle)
	assert.EqualValues(testSettings.Jira.BasicAuthConfig.Username, dbSettings.Jira.BasicAuthConfig.Username)
	assert.EqualValues(testSettings.LoggerConfig.DefaultLevel, dbSettings.LoggerConfig.DefaultLevel)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.Count, dbSettings.LoggerConfig.Buffer.Count)
	assert.EqualValues(testSettings.Notify.SMTP.From, dbSettings.Notify.SMTP.From)
	assert.EqualValues(testSettings.Notify.SMTP.Port, dbSettings.Notify.SMTP.Port)
	assert.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(dbSettings.Notify.SMTP.AdminEmail))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Name, dbSettings.Providers.AWS.EC2Keys[0].Name)
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Key, dbSettings.Providers.AWS.EC2Keys[0].Key)
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Secret, dbSettings.Providers.AWS.EC2Keys[0].Secret)
	assert.EqualValues(testSettings.Providers.AWS.DefaultSecurityGroup, dbSettings.Providers.AWS.DefaultSecurityGroup)
	assert.EqualValues(testSettings.Providers.AWS.MaxVolumeSizePerUser, dbSettings.Providers.AWS.MaxVolumeSizePerUser)
	assert.EqualValues(testSettings.Providers.AWS.S3.Key, dbSettings.Providers.AWS.S3.Key)
	assert.EqualValues(testSettings.Providers.AWS.S3.Secret, dbSettings.Providers.AWS.S3.Secret)
	assert.EqualValues(testSettings.Providers.AWS.S3.Bucket, dbSettings.Providers.AWS.S3.Bucket)
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Key, dbSettings.Providers.AWS.TaskSync.Key)
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Secret, dbSettings.Providers.AWS.TaskSync.Secret)
	assert.EqualValues(testSettings.Providers.AWS.TaskSync.Bucket, dbSettings.Providers.AWS.TaskSync.Bucket)
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Key, dbSettings.Providers.AWS.TaskSyncRead.Key)
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Secret, dbSettings.Providers.AWS.TaskSyncRead.Secret)
	assert.EqualValues(testSettings.Providers.AWS.TaskSyncRead.Bucket, dbSettings.Providers.AWS.TaskSyncRead.Bucket)
	assert.EqualValues(testSettings.Providers.Docker.APIVersion, dbSettings.Providers.Docker.APIVersion)
	assert.EqualValues(testSettings.Providers.GCE.ClientEmail, dbSettings.Providers.GCE.ClientEmail)
	assert.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, dbSettings.Providers.OpenStack.IdentityEndpoint)
	assert.EqualValues(testSettings.Providers.VSphere.Host, dbSettings.Providers.VSphere.Host)
	assert.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, dbSettings.RepoTracker.MaxConcurrentRequests)
	assert.EqualValues(testSettings.Scheduler.TaskFinder, dbSettings.Scheduler.TaskFinder)
	assert.EqualValues(testSettings.ServiceFlags.HostInitDisabled, dbSettings.ServiceFlags.HostInitDisabled)
	assert.EqualValues(testSettings.ServiceFlags.S3BinaryDownloadsDisabled, dbSettings.ServiceFlags.S3BinaryDownloadsDisabled)
	require.Len(dbSettings.SSHKeyPairs, len(testSettings.SSHKeyPairs))
	for i := 0; i < len(testSettings.SSHKeyPairs); i++ {
		assert.Equal(dbSettings.SSHKeyPairs[i].Name, testSettings.SSHKeyPairs[i].Name)
		assert.Equal(dbSettings.SSHKeyPairs[i].Public, testSettings.SSHKeyPairs[i].Public)
		assert.Equal(dbSettings.SSHKeyPairs[i].Private, testSettings.SSHKeyPairs[i].Private)
	}
	assert.EqualValues(testSettings.Slack.Level, dbSettings.Slack.Level)
	assert.EqualValues(testSettings.Slack.Options.Channel, dbSettings.Slack.Options.Channel)
	assert.EqualValues(testSettings.Splunk.Channel, dbSettings.Splunk.Channel)
	assert.EqualValues(testSettings.Triggers.GenerateTaskDistro, dbSettings.Triggers.GenerateTaskDistro)
	assert.EqualValues(testSettings.Ui.HttpListenAddr, dbSettings.Ui.HttpListenAddr)
	assert.EqualValues(testSettings.Spawnhost.SpawnHostsPerUser, dbSettings.Spawnhost.SpawnHostsPerUser)
	assert.EqualValues(testSettings.Spawnhost.UnexpirableHostsPerUser, dbSettings.Spawnhost.UnexpirableHostsPerUser)
	assert.EqualValues(testSettings.Spawnhost.UnexpirableVolumesPerUser, dbSettings.Spawnhost.UnexpirableVolumesPerUser)

}

func TestRestart(t *testing.T) {
	assert := assert.New(t)
	restartResp := &RestartResponse{
		ItemsRestarted: []string{"task1", "task2", "task3"},
		ItemsErrored:   []string{"task4", "task5"},
	}

	apiResp := RestartResponse{}
	assert.NoError(apiResp.BuildFromService(restartResp))
	assert.Equal(3, len(apiResp.ItemsRestarted))
	assert.Equal(2, len(apiResp.ItemsErrored))
}

func TestEventConversion(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()
	evt := event.EventLogEntry{
		Timestamp: now,
		Data: &event.AdminEventData{
			User:    "me",
			Section: "global",
			GUID:    "abc",
			Changes: event.ConfigDataChange{
				Before: &evergreen.Settings{},
				After:  &evergreen.Settings{Banner: "banner"},
			},
		},
	}
	apiEvent := APIAdminEvent{}
	assert.NoError(apiEvent.BuildFromService(evt))
	assert.EqualValues(evt.Timestamp, *apiEvent.Timestamp)
	assert.EqualValues("me", apiEvent.User)
	assert.NotEmpty(apiEvent.Guid)
	before := apiEvent.Before.(*APIAdminSettings)
	after := apiEvent.After.(*APIAdminSettings)
	assert.EqualValues("", *before.Banner)
	assert.EqualValues("banner", *after.Banner)
}

func TestAPIServiceFlagsModelInterface(t *testing.T) {
	assert := assert.New(t)

	flags := evergreen.ServiceFlags{}
	assert.NotPanics(func() {
		v := reflect.ValueOf(&flags).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Kind() == reflect.Bool {
				f.SetBool(true)
				assert.True(f.Bool())
			}
		}
	}, "error setting all fields to true")

	apiFlags := APIServiceFlags{}
	assert.NoError(apiFlags.BuildFromService(flags))
	allStructFieldsTrue(t, &apiFlags)

	newFlagsI, err := apiFlags.ToService()
	assert.NoError(err)
	newFlags, ok := newFlagsI.(evergreen.ServiceFlags)
	require.True(t, ok)
	allStructFieldsTrue(t, &newFlags)
}

func allStructFieldsTrue(t *testing.T, s interface{}) {
	elem := reflect.ValueOf(s).Elem()
	for i := 0; i < elem.NumField(); i++ {
		f := elem.Field(i)
		if f.Kind() == reflect.Bool {
			if !f.Bool() {
				t.Errorf("all fields should be true, but '%s' was false", reflect.TypeOf(s).Elem().Field(i).Name)
			}
		}
	}
}

func TestAPIJIRANotificationsConfig(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	api := APIJIRANotificationsConfig{}
	dbModelIface, err := api.ToService()
	assert.NoError(err)
	dbModel, ok := dbModelIface.(evergreen.JIRANotificationsConfig)
	assert.True(ok)
	assert.Nil(dbModel.CustomFields)

	api = APIJIRANotificationsConfig{
		CustomFields: map[string]APIJIRANotificationsProject{
			"EVG": APIJIRANotificationsProject{
				Fields: map[string]string{
					"customfield_12345": "{{.Something}}",
					"customfield_12346": "{{.SomethingElse}}",
				},
				Components: []string{"component0", "component1"},
			},
			"GVE": APIJIRANotificationsProject{
				Fields: map[string]string{
					"customfield_54321": "{{.SomethingElser}}",
					"customfield_54322": "{{.SomethingEvenElser}}",
				},
				Labels: []string{"label0", "label1"},
			},
		},
	}

	dbModelIface, err = api.ToService()
	assert.NoError(err)
	dbModel, ok = dbModelIface.(evergreen.JIRANotificationsConfig)
	assert.True(ok)
	assert.Len(dbModel.CustomFields, 2)
	for _, project := range dbModel.CustomFields {
		if project.Project == "EVG" {
			assert.Len(project.Fields, 2)
			for _, field := range project.Fields {
				if field.Field == "customfield_12345" {
					assert.Equal("{{.Something}}", field.Template)
				} else if field.Field == "customfield_12346" {
					assert.Equal("{{.SomethingElse}}", field.Template)
				} else {
					assert.Failf("unexpected field name '%s", field.Field)
				}
			}
			require.Len(project.Components, 2)
			assert.Equal("component0", project.Components[0])
			assert.Equal("component1", project.Components[1])
		} else if project.Project == "GVE" {
			assert.Len(project.Fields, 2)
			for _, field := range project.Fields {
				if field.Field == "customfield_54321" {
					assert.Equal("{{.SomethingElser}}", field.Template)
				} else if field.Field == "customfield_54322" {
					assert.Equal("{{.SomethingEvenElser}}", field.Template)
				} else {
					assert.Failf("unexpected field name '%s", field.Field)
				}
			}
			require.Len(project.Labels, 2)
			assert.Equal("label0", project.Labels[0])
			assert.Equal("label1", project.Labels[1])
		} else {
			assert.Failf("unexpected project name '%s'", project.Project)
		}
	}

	newAPI := APIJIRANotificationsConfig{}
	assert.NoError(newAPI.BuildFromService(&dbModel))
	assert.Equal(api, newAPI)
}
