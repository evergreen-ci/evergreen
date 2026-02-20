package model

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		assert.True(ok, "config field '%s' is missing from evergreen.Settings", configFieldName)
		if ok {
			matched[configFieldName] = false
		}
	}

	exclude := []string{"Id", "CredentialsNew", "Database", "KeysNew", "ExpansionsNew", "PluginsNew"}
	for k, v := range matched {
		if !utility.StringSliceContains(exclude, k) {
			assert.False(v, "config field '%s' is missing from APIAdminSettings", k)
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
	assert.Equal(testSettings.AWSInstanceRole, *apiSettings.AWSInstanceRole)
	assert.Equal(testSettings.Banner, *apiSettings.Banner)
	assert.EqualValues(testSettings.BannerTheme, *apiSettings.BannerTheme)
	assert.Equal(testSettings.ConfigDir, *apiSettings.ConfigDir)
	assert.Equal(testSettings.GithubPRCreatorOrg, *apiSettings.GithubPRCreatorOrg)
	assert.Equal(testSettings.LogPath, *apiSettings.LogPath)
	assert.Equal(testSettings.PprofPort, *apiSettings.PprofPort)

	for k, v := range testSettings.Expansions {
		assert.Contains(apiSettings.Expansions, k)
		assert.Equal(v, apiSettings.Expansions[k])
	}
	for k, v := range testSettings.Plugins {
		assert.Contains(apiSettings.Plugins, k)
		for k2, v2 := range v {
			assert.Contains(apiSettings.Plugins[k], k2)
			assert.Equal(v2, apiSettings.Plugins[k][k2])
		}
	}
	assert.Equal(testSettings.ShutdownWaitSeconds, *apiSettings.ShutdownWaitSeconds)

	assert.EqualValues(testSettings.Amboy.Name, utility.FromStringPtr(apiSettings.Amboy.Name))
	assert.EqualValues(testSettings.Amboy.LocalStorage, apiSettings.Amboy.LocalStorage)
	assert.EqualValues(testSettings.Amboy.GroupDefaultWorkers, apiSettings.Amboy.GroupDefaultWorkers)
	assert.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, apiSettings.Amboy.GroupBackgroundCreateFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, apiSettings.Amboy.GroupPruneFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupTTLMinutes, apiSettings.Amboy.GroupTTLMinutes)
	assert.EqualValues(testSettings.Amboy.LockTimeoutMinutes, apiSettings.Amboy.LockTimeoutMinutes)
	assert.EqualValues(testSettings.Amboy.SampleSize, apiSettings.Amboy.SampleSize)
	assert.EqualValues(testSettings.Amboy.Retry, apiSettings.Amboy.Retry)
	require.Equal(len(testSettings.Amboy.NamedQueues), len(apiSettings.Amboy.NamedQueues))
	for i := range testSettings.Amboy.NamedQueues {
		assert.Equal(testSettings.Amboy.NamedQueues[i].Name, utility.FromStringPtr(apiSettings.Amboy.NamedQueues[i].Name))
		assert.Equal(testSettings.Amboy.NamedQueues[i].Regexp, utility.FromStringPtr(apiSettings.Amboy.NamedQueues[i].Regexp))
		assert.Equal(testSettings.Amboy.NamedQueues[i].NumWorkers, apiSettings.Amboy.NamedQueues[i].NumWorkers)
		assert.Equal(testSettings.Amboy.NamedQueues[i].SampleSize, apiSettings.Amboy.NamedQueues[i].SampleSize)
		assert.Equal(testSettings.Amboy.NamedQueues[i].LockTimeoutSeconds, apiSettings.Amboy.NamedQueues[i].LockTimeoutSeconds)
	}
	assert.EqualValues(testSettings.AmboyDB.URL, utility.FromStringPtr(apiSettings.AmboyDB.URL))
	assert.EqualValues(testSettings.AmboyDB.Database, utility.FromStringPtr(apiSettings.AmboyDB.Database))
	assert.EqualValues(testSettings.Api.HttpListenAddr, utility.FromStringPtr(apiSettings.Api.HttpListenAddr))
	assert.EqualValues(testSettings.Api.URL, utility.FromStringPtr(apiSettings.Api.URL))
	assert.EqualValues(testSettings.Api.CorpURL, utility.FromStringPtr(apiSettings.Api.CorpURL))
	assert.EqualValues(testSettings.AuthConfig.PreferredType, utility.FromStringPtr(apiSettings.AuthConfig.PreferredType))
	assert.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, utility.FromStringPtr(apiSettings.AuthConfig.Naive.Users[0].Username))
	assert.EqualValues(testSettings.AuthConfig.Okta.ClientID, utility.FromStringPtr(apiSettings.AuthConfig.Okta.ClientID))
	assert.EqualValues(testSettings.AuthConfig.Github.ClientId, utility.FromStringPtr(apiSettings.AuthConfig.Github.ClientId))
	assert.EqualValues(testSettings.AuthConfig.Multi.ReadWrite[0], apiSettings.AuthConfig.Multi.ReadWrite[0])
	assert.EqualValues(testSettings.AuthConfig.Kanopy.Issuer, utility.FromStringPtr(apiSettings.AuthConfig.Kanopy.Issuer))
	assert.EqualValues(testSettings.AuthConfig.OAuth.Issuer, utility.FromStringPtr(apiSettings.AuthConfig.OAuth.Issuer))
	assert.EqualValues(testSettings.AuthConfig.OAuth.ClientID, utility.FromStringPtr(apiSettings.AuthConfig.OAuth.ClientID))
	assert.EqualValues(testSettings.AuthConfig.OAuth.ConnectorID, utility.FromStringPtr(apiSettings.AuthConfig.OAuth.ConnectorID))
	assert.Equal(len(testSettings.AuthConfig.Github.Users), len(apiSettings.AuthConfig.Github.Users))
	assert.Equal(testSettings.Buckets.LogBucket.Name, utility.FromStringPtr(apiSettings.Buckets.LogBucket.Name))
	assert.EqualValues(testSettings.Buckets.LogBucket.Type, utility.FromStringPtr(apiSettings.Buckets.LogBucket.Type))
	assert.Equal(testSettings.Buckets.LogBucket.DBName, utility.FromStringPtr(apiSettings.Buckets.LogBucket.DBName))
	assert.EqualValues(testSettings.Buckets.Credentials.Key, utility.FromStringPtr(apiSettings.Buckets.Credentials.Key))
	assert.EqualValues(testSettings.Buckets.Credentials.Secret, utility.FromStringPtr(apiSettings.Buckets.Credentials.Secret))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Distro, utility.FromStringPtr(apiSettings.ContainerPools.Pools[0].Distro))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Id, utility.FromStringPtr(apiSettings.ContainerPools.Pools[0].Id))
	assert.EqualValues(testSettings.ContainerPools.Pools[0].MaxContainers, apiSettings.ContainerPools.Pools[0].MaxContainers)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Port, apiSettings.ContainerPools.Pools[0].Port)
	assert.EqualValues(testSettings.FWS.URL, utility.FromStringPtr(apiSettings.FWS.URL))
	assert.EqualValues(testSettings.Graphite.CIOptimizationToken, utility.FromStringPtr(apiSettings.Graphite.CIOptimizationToken))
	assert.EqualValues(testSettings.Graphite.ServerURL, utility.FromStringPtr(apiSettings.Graphite.ServerURL))
	assert.Equal(testSettings.HostJasper.BinaryName, utility.FromStringPtr(apiSettings.HostJasper.BinaryName))
	assert.Equal(testSettings.HostJasper.DownloadFileName, utility.FromStringPtr(apiSettings.HostJasper.DownloadFileName))
	assert.Equal(testSettings.HostJasper.Port, apiSettings.HostJasper.Port)
	assert.Equal(testSettings.HostJasper.URL, utility.FromStringPtr(apiSettings.HostJasper.URL))
	assert.Equal(testSettings.HostJasper.Version, utility.FromStringPtr(apiSettings.HostJasper.Version))
	assert.EqualValues(testSettings.HostInit.HostThrottle, apiSettings.HostInit.HostThrottle)
	assert.EqualValues(testSettings.HostInit.ProvisioningThrottle, apiSettings.HostInit.ProvisioningThrottle)
	assert.EqualValues(testSettings.HostInit.CloudStatusBatchSize, apiSettings.HostInit.CloudStatusBatchSize)
	assert.EqualValues(testSettings.HostInit.MaxTotalDynamicHosts, apiSettings.HostInit.MaxTotalDynamicHosts)
	assert.EqualValues(testSettings.Jira.PersonalAccessToken, utility.FromStringPtr(apiSettings.Jira.PersonalAccessToken))
	assert.EqualValues(testSettings.LoggerConfig.DefaultLevel, utility.FromStringPtr(apiSettings.LoggerConfig.DefaultLevel))
	assert.EqualValues(testSettings.LoggerConfig.Buffer.Count, apiSettings.LoggerConfig.Buffer.Count)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, apiSettings.LoggerConfig.Buffer.UseAsync)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, apiSettings.LoggerConfig.Buffer.IncomingBufferFactor)
	assert.EqualValues(testSettings.Notify.SES.SenderAddress, utility.FromStringPtr(apiSettings.Notify.SES.SenderAddress))
	assert.EqualValues(testSettings.Overrides.Overrides[0].SectionID, utility.FromStringPtr(apiSettings.Overrides.Overrides[0].SectionID))
	assert.EqualValues(testSettings.Overrides.Overrides[0].Field, utility.FromStringPtr(apiSettings.Overrides.Overrides[0].Field))
	assert.EqualValues(testSettings.Overrides.Overrides[0].Value, apiSettings.Overrides.Overrides[0].Value)
	assert.EqualValues(testSettings.ParameterStore.Prefix, utility.FromStringPtr(apiSettings.ParameterStore.Prefix))
	assert.EqualValues(testSettings.ProjectCreation.TotalProjectLimit, apiSettings.ProjectCreation.TotalProjectLimit)
	assert.EqualValues(testSettings.ProjectCreation.RepoProjectLimit, apiSettings.ProjectCreation.RepoProjectLimit)
	assert.EqualValues(testSettings.ProjectCreation.JiraProject, apiSettings.ProjectCreation.JiraProject)
	assert.EqualValues(testSettings.ProjectCreation.RepoExceptions[0].Owner, utility.FromStringPtr(apiSettings.ProjectCreation.RepoExceptions[0].Owner))
	assert.EqualValues(testSettings.ProjectCreation.RepoExceptions[0].Repo, utility.FromStringPtr(apiSettings.ProjectCreation.RepoExceptions[0].Repo))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Name, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Name))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Key, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Key))
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Secret, utility.FromStringPtr(apiSettings.Providers.AWS.EC2Keys[0].Secret))
	assert.EqualValues(testSettings.Providers.AWS.DefaultSecurityGroup, utility.FromStringPtr(apiSettings.Providers.AWS.DefaultSecurityGroup))
	assert.EqualValues(testSettings.Providers.AWS.MaxVolumeSizePerUser, *apiSettings.Providers.AWS.MaxVolumeSizePerUser)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Key, utility.FromStringPtr(apiSettings.Providers.AWS.ParserProject.Key))
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Secret, utility.FromStringPtr(apiSettings.Providers.AWS.ParserProject.Secret))
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Bucket, utility.FromStringPtr(apiSettings.Providers.AWS.ParserProject.Bucket))
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Prefix, utility.FromStringPtr(apiSettings.Providers.AWS.ParserProject.Prefix))
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix, utility.FromStringPtr(apiSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix))
	assert.EqualValues(testSettings.Providers.AWS.PersistentDNS.HostedZoneID, utility.FromStringPtr(apiSettings.Providers.AWS.PersistentDNS.HostedZoneID))
	assert.EqualValues(testSettings.Providers.AWS.PersistentDNS.Domain, utility.FromStringPtr(apiSettings.Providers.AWS.PersistentDNS.Domain))
	require.Len(apiSettings.Providers.AWS.AccountRoles, len(testSettings.Providers.AWS.AccountRoles))
	for i, ar := range testSettings.Providers.AWS.AccountRoles {
		assert.Equal(ar.Account, utility.FromStringPtr(apiSettings.Providers.AWS.AccountRoles[i].Account))
		assert.Equal(ar.Role, utility.FromStringPtr(apiSettings.Providers.AWS.AccountRoles[i].Role))
	}
	assert.EqualValues(testSettings.Providers.Docker.APIVersion, utility.FromStringPtr(apiSettings.Providers.Docker.APIVersion))
	assert.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, apiSettings.RepoTracker.MaxConcurrentRequests)
	assert.EqualValues(testSettings.Scheduler.TaskFinder, utility.FromStringPtr(apiSettings.Scheduler.TaskFinder))
	assert.EqualValues(testSettings.ServiceFlags.HostInitDisabled, apiSettings.ServiceFlags.HostInitDisabled)
	assert.EqualValues(testSettings.ServiceFlags.LargeParserProjectsDisabled, apiSettings.ServiceFlags.LargeParserProjectsDisabled)
	assert.EqualValues(testSettings.ServiceFlags.SleepScheduleDisabled, apiSettings.ServiceFlags.SleepScheduleDisabled)
	assert.EqualValues(testSettings.ServiceFlags.StaticAPIKeysDisabled, apiSettings.ServiceFlags.StaticAPIKeysDisabled)
	assert.EqualValues(testSettings.ServiceFlags.JWTTokenForCLIDisabled, apiSettings.ServiceFlags.JWTTokenForCLIDisabled)
	assert.EqualValues(testSettings.ServiceFlags.SystemFailedTaskRestartDisabled, apiSettings.ServiceFlags.SystemFailedTaskRestartDisabled)
	assert.EqualValues(testSettings.ServiceFlags.CPUDegradedModeDisabled, apiSettings.ServiceFlags.DegradedModeDisabled)
	assert.EqualValues(testSettings.ServiceFlags.ElasticIPsDisabled, apiSettings.ServiceFlags.ElasticIPsDisabled)
	assert.EqualValues(testSettings.SingleTaskDistro.ProjectTasksPairs[0].ProjectID, apiSettings.SingleTaskDistro.ProjectTasksPairs[0].ProjectID)
	assert.ElementsMatch(testSettings.SingleTaskDistro.ProjectTasksPairs[0].AllowedTasks, apiSettings.SingleTaskDistro.ProjectTasksPairs[0].AllowedTasks)
	assert.EqualValues(testSettings.Slack.Level, utility.FromStringPtr(apiSettings.Slack.Level))
	assert.EqualValues(testSettings.Slack.Options.Channel, utility.FromStringPtr(apiSettings.Slack.Options.Channel))
	assert.ElementsMatch(testSettings.SleepSchedule.PermanentlyExemptHosts, apiSettings.SleepSchedule.PermanentlyExemptHosts)
	assert.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, utility.FromStringPtr(apiSettings.Splunk.SplunkConnectionInfo.Channel))
	assert.EqualValues(testSettings.SSH.SpawnHostKey.Name, utility.FromStringPtr(apiSettings.SSH.SpawnHostKey.Name))
	assert.EqualValues(testSettings.SSH.SpawnHostKey.SecretARN, utility.FromStringPtr(apiSettings.SSH.SpawnHostKey.SecretARN))
	assert.EqualValues(testSettings.SSH.TaskHostKey.Name, utility.FromStringPtr(apiSettings.SSH.TaskHostKey.Name))
	assert.EqualValues(testSettings.SSH.TaskHostKey.SecretARN, utility.FromStringPtr(apiSettings.SSH.TaskHostKey.SecretARN))
	assert.EqualValues(testSettings.TaskLimits.MaxTasksPerVersion, utility.FromIntPtr(apiSettings.TaskLimits.MaxTasksPerVersion))
	assert.EqualValues(testSettings.TestSelection.URL, utility.FromStringPtr(apiSettings.TestSelection.URL))
	assert.EqualValues(testSettings.Triggers.GenerateTaskDistro, utility.FromStringPtr(apiSettings.Triggers.GenerateTaskDistro))
	assert.EqualValues(testSettings.Ui.HttpListenAddr, utility.FromStringPtr(apiSettings.Ui.HttpListenAddr))
	assert.EqualValues(testSettings.Ui.StagingEnvironment, utility.FromStringPtr(apiSettings.Ui.StagingEnvironment))
	assert.Equal(testSettings.Spawnhost.SpawnHostsPerUser, *apiSettings.Spawnhost.SpawnHostsPerUser)
	assert.Equal(testSettings.Spawnhost.UnexpirableHostsPerUser, *apiSettings.Spawnhost.UnexpirableHostsPerUser)
	assert.Equal(testSettings.Spawnhost.UnexpirableVolumesPerUser, *apiSettings.Spawnhost.UnexpirableVolumesPerUser)
	assert.Equal(testSettings.DebugSpawnHosts.SetupScript, utility.FromStringPtr(apiSettings.DebugSpawnHosts.SetupScript))
	assert.Equal(testSettings.Tracer.Enabled, *apiSettings.Tracer.Enabled)
	assert.Equal(testSettings.Tracer.CollectorEndpoint, *apiSettings.Tracer.CollectorEndpoint)
	assert.Equal(testSettings.Tracer.CollectorInternalEndpoint, *apiSettings.Tracer.CollectorInternalEndpoint)
	assert.Equal(testSettings.GitHubCheckRun.CheckRunLimit, *apiSettings.GitHubCheckRun.CheckRunLimit)
	assert.Equal(testSettings.Sage.BaseURL, utility.FromStringPtr(apiSettings.Sage.BaseURL))

	// test converting from the API model back to a DB model
	dbInterface, err := apiSettings.ToService()
	assert.NoError(err)
	dbSettings := dbInterface.(evergreen.Settings)
	assert.EqualValues(testSettings.Amboy.Name, dbSettings.Amboy.Name)
	assert.EqualValues(testSettings.AmboyDB, dbSettings.AmboyDB)
	assert.EqualValues(testSettings.Amboy.LocalStorage, dbSettings.Amboy.LocalStorage)
	assert.EqualValues(testSettings.Amboy.GroupDefaultWorkers, dbSettings.Amboy.GroupDefaultWorkers)
	assert.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, dbSettings.Amboy.GroupBackgroundCreateFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, dbSettings.Amboy.GroupPruneFrequencyMinutes)
	assert.EqualValues(testSettings.Amboy.GroupTTLMinutes, dbSettings.Amboy.GroupTTLMinutes)
	assert.EqualValues(testSettings.Amboy.LockTimeoutMinutes, dbSettings.Amboy.LockTimeoutMinutes)
	assert.EqualValues(testSettings.Amboy.SampleSize, dbSettings.Amboy.SampleSize)
	assert.EqualValues(testSettings.Amboy.Retry, dbSettings.Amboy.Retry)
	require.Equal(len(testSettings.Amboy.NamedQueues), len(dbSettings.Amboy.NamedQueues))
	for i := range testSettings.Amboy.NamedQueues {
		assert.Equal(testSettings.Amboy.NamedQueues[i].Name, dbSettings.Amboy.NamedQueues[i].Name)
		assert.Equal(testSettings.Amboy.NamedQueues[i].Regexp, dbSettings.Amboy.NamedQueues[i].Regexp)
		assert.Equal(testSettings.Amboy.NamedQueues[i].NumWorkers, dbSettings.Amboy.NamedQueues[i].NumWorkers)
		assert.Equal(testSettings.Amboy.NamedQueues[i].SampleSize, dbSettings.Amboy.NamedQueues[i].SampleSize)
		assert.Equal(testSettings.Amboy.NamedQueues[i].LockTimeoutSeconds, dbSettings.Amboy.NamedQueues[i].LockTimeoutSeconds)
	}
	assert.EqualValues(testSettings.AmboyDB.URL, dbSettings.AmboyDB.URL)
	assert.EqualValues(testSettings.AmboyDB.Database, dbSettings.AmboyDB.Database)
	assert.EqualValues(testSettings.Api.HttpListenAddr, dbSettings.Api.HttpListenAddr)
	assert.EqualValues(testSettings.Api.URL, dbSettings.Api.URL)
	assert.EqualValues(testSettings.Api.CorpURL, dbSettings.Api.CorpURL)
	assert.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, dbSettings.AuthConfig.Naive.Users[0].Username)
	assert.EqualValues(testSettings.AuthConfig.Github.ClientId, dbSettings.AuthConfig.Github.ClientId)
	assert.Equal(len(testSettings.AuthConfig.Github.Users), len(dbSettings.AuthConfig.Github.Users))
	assert.EqualValues(testSettings.AuthConfig.Multi.ReadWrite[0], dbSettings.AuthConfig.Multi.ReadWrite[0])
	assert.EqualValues(testSettings.AuthConfig.Kanopy.Issuer, dbSettings.AuthConfig.Kanopy.Issuer)
	assert.Equal(testSettings.Buckets.LogBucket.Name, utility.FromStringPtr(apiSettings.Buckets.LogBucket.Name))
	assert.EqualValues(testSettings.Buckets.LogBucket.Type, utility.FromStringPtr(apiSettings.Buckets.LogBucket.Type))
	assert.Equal(testSettings.Buckets.LogBucket.DBName, utility.FromStringPtr(apiSettings.Buckets.LogBucket.DBName))
	assert.EqualValues(testSettings.Buckets.Credentials.Key, dbSettings.Buckets.Credentials.Key)
	assert.EqualValues(testSettings.Buckets.Credentials.Secret, dbSettings.Buckets.Credentials.Secret)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Distro, dbSettings.ContainerPools.Pools[0].Distro)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Id, dbSettings.ContainerPools.Pools[0].Id)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].MaxContainers, dbSettings.ContainerPools.Pools[0].MaxContainers)
	assert.EqualValues(testSettings.ContainerPools.Pools[0].Port, dbSettings.ContainerPools.Pools[0].Port)
	assert.EqualValues(testSettings.FWS.URL, dbSettings.FWS.URL)
	assert.EqualValues(testSettings.Graphite.CIOptimizationToken, dbSettings.Graphite.CIOptimizationToken)
	assert.EqualValues(testSettings.Graphite.ServerURL, dbSettings.Graphite.ServerURL)
	assert.EqualValues(testSettings.HostInit.HostThrottle, dbSettings.HostInit.HostThrottle)
	assert.EqualValues(testSettings.Jira.PersonalAccessToken, dbSettings.Jira.PersonalAccessToken)
	assert.EqualValues(testSettings.LoggerConfig.DefaultLevel, dbSettings.LoggerConfig.DefaultLevel)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.Count, dbSettings.LoggerConfig.Buffer.Count)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, dbSettings.LoggerConfig.Buffer.UseAsync)
	assert.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, dbSettings.LoggerConfig.Buffer.IncomingBufferFactor)
	assert.EqualValues(testSettings.Notify.SES.SenderAddress, dbSettings.Notify.SES.SenderAddress)
	assert.EqualValues(testSettings.Overrides.Overrides[0].SectionID, dbSettings.Overrides.Overrides[0].SectionID)
	assert.EqualValues(testSettings.Overrides.Overrides[0].Field, dbSettings.Overrides.Overrides[0].Field)
	assert.EqualValues(testSettings.Overrides.Overrides[0].Value, dbSettings.Overrides.Overrides[0].Value)
	assert.EqualValues(testSettings.ProjectCreation.TotalProjectLimit, dbSettings.ProjectCreation.TotalProjectLimit)
	assert.EqualValues(testSettings.ProjectCreation.RepoProjectLimit, dbSettings.ProjectCreation.RepoProjectLimit)
	assert.EqualValues(testSettings.ProjectCreation.JiraProject, dbSettings.ProjectCreation.JiraProject)
	assert.EqualValues(testSettings.ProjectCreation.RepoExceptions[0].Owner, dbSettings.ProjectCreation.RepoExceptions[0].Owner)
	assert.EqualValues(testSettings.ProjectCreation.RepoExceptions[0].Repo, dbSettings.ProjectCreation.RepoExceptions[0].Repo)
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Name, dbSettings.Providers.AWS.EC2Keys[0].Name)
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Key, dbSettings.Providers.AWS.EC2Keys[0].Key)
	assert.EqualValues(testSettings.Providers.AWS.EC2Keys[0].Secret, dbSettings.Providers.AWS.EC2Keys[0].Secret)
	assert.EqualValues(testSettings.Providers.AWS.DefaultSecurityGroup, dbSettings.Providers.AWS.DefaultSecurityGroup)
	assert.EqualValues(testSettings.Providers.AWS.MaxVolumeSizePerUser, dbSettings.Providers.AWS.MaxVolumeSizePerUser)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Key, dbSettings.Providers.AWS.ParserProject.Key)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Secret, dbSettings.Providers.AWS.ParserProject.Secret)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Bucket, dbSettings.Providers.AWS.ParserProject.Bucket)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.Prefix, dbSettings.Providers.AWS.ParserProject.Prefix)
	assert.EqualValues(testSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix, dbSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix)
	assert.EqualValues(testSettings.Providers.AWS.PersistentDNS.HostedZoneID, dbSettings.Providers.AWS.PersistentDNS.HostedZoneID)
	assert.EqualValues(testSettings.Providers.AWS.PersistentDNS.Domain, dbSettings.Providers.AWS.PersistentDNS.Domain)
	assert.EqualValues(testSettings.Providers.Docker.APIVersion, dbSettings.Providers.Docker.APIVersion)
	assert.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, dbSettings.RepoTracker.MaxConcurrentRequests)
	assert.EqualValues(testSettings.Scheduler.TaskFinder, dbSettings.Scheduler.TaskFinder)
	assert.EqualValues(testSettings.ServiceFlags.HostInitDisabled, dbSettings.ServiceFlags.HostInitDisabled)
	assert.EqualValues(testSettings.ServiceFlags.LargeParserProjectsDisabled, dbSettings.ServiceFlags.LargeParserProjectsDisabled)
	assert.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, dbSettings.ServiceFlags.CloudCleanupDisabled)
	assert.EqualValues(testSettings.ServiceFlags.SleepScheduleDisabled, dbSettings.ServiceFlags.SleepScheduleDisabled)
	assert.EqualValues(testSettings.ServiceFlags.StaticAPIKeysDisabled, dbSettings.ServiceFlags.StaticAPIKeysDisabled)
	assert.EqualValues(testSettings.ServiceFlags.JWTTokenForCLIDisabled, dbSettings.ServiceFlags.JWTTokenForCLIDisabled)
	assert.EqualValues(testSettings.ServiceFlags.SystemFailedTaskRestartDisabled, apiSettings.ServiceFlags.SystemFailedTaskRestartDisabled)
	assert.EqualValues(testSettings.ServiceFlags.CPUDegradedModeDisabled, apiSettings.ServiceFlags.DegradedModeDisabled)
	assert.EqualValues(testSettings.ServiceFlags.ElasticIPsDisabled, apiSettings.ServiceFlags.ElasticIPsDisabled)
	assert.EqualValues(testSettings.SingleTaskDistro.ProjectTasksPairs[0].ProjectID, dbSettings.SingleTaskDistro.ProjectTasksPairs[0].ProjectID)
	assert.ElementsMatch(testSettings.SingleTaskDistro.ProjectTasksPairs[0].AllowedTasks, dbSettings.SingleTaskDistro.ProjectTasksPairs[0].AllowedTasks)
	assert.EqualValues(testSettings.Slack.Level, dbSettings.Slack.Level)
	assert.EqualValues(testSettings.Slack.Options.Channel, dbSettings.Slack.Options.Channel)
	assert.ElementsMatch(testSettings.SleepSchedule.PermanentlyExemptHosts, apiSettings.SleepSchedule.PermanentlyExemptHosts)
	assert.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, dbSettings.Splunk.SplunkConnectionInfo.Channel)
	assert.EqualValues(testSettings.SSH.SpawnHostKey.Name, dbSettings.SSH.SpawnHostKey.Name)
	assert.EqualValues(testSettings.SSH.SpawnHostKey.SecretARN, dbSettings.SSH.SpawnHostKey.SecretARN)
	assert.EqualValues(testSettings.SSH.TaskHostKey.Name, dbSettings.SSH.TaskHostKey.Name)
	assert.EqualValues(testSettings.SSH.TaskHostKey.SecretARN, dbSettings.SSH.TaskHostKey.SecretARN)
	assert.EqualValues(testSettings.TaskLimits.MaxTasksPerVersion, dbSettings.TaskLimits.MaxTasksPerVersion)
	assert.EqualValues(testSettings.TestSelection.URL, dbSettings.TestSelection.URL)
	assert.EqualValues(testSettings.Triggers.GenerateTaskDistro, dbSettings.Triggers.GenerateTaskDistro)
	assert.EqualValues(testSettings.Ui.HttpListenAddr, dbSettings.Ui.HttpListenAddr)
	assert.EqualValues(testSettings.Ui.StagingEnvironment, dbSettings.Ui.StagingEnvironment)
	assert.EqualValues(testSettings.Spawnhost.SpawnHostsPerUser, dbSettings.Spawnhost.SpawnHostsPerUser)
	assert.EqualValues(testSettings.Spawnhost.UnexpirableHostsPerUser, dbSettings.Spawnhost.UnexpirableHostsPerUser)
	assert.EqualValues(testSettings.Spawnhost.UnexpirableVolumesPerUser, dbSettings.Spawnhost.UnexpirableVolumesPerUser)
	assert.EqualValues(testSettings.DebugSpawnHosts.SetupScript, dbSettings.DebugSpawnHosts.SetupScript)
	assert.EqualValues(testSettings.Tracer.Enabled, dbSettings.Tracer.Enabled)
	assert.EqualValues(testSettings.Tracer.CollectorEndpoint, dbSettings.Tracer.CollectorEndpoint)
	assert.EqualValues(testSettings.Tracer.CollectorInternalEndpoint, dbSettings.Tracer.CollectorInternalEndpoint)
	assert.EqualValues(testSettings.GitHubCheckRun.CheckRunLimit, dbSettings.GitHubCheckRun.CheckRunLimit)
	assert.EqualValues(testSettings.Sage.BaseURL, dbSettings.Sage.BaseURL)
}

func TestRestart(t *testing.T) {
	assert := assert.New(t)
	restartResp := &RestartResponse{
		ItemsRestarted: []string{"task1", "task2", "task3"},
		ItemsErrored:   []string{"task4", "task5"},
	}

	apiResp := RestartResponse{}
	assert.NoError(apiResp.BuildFromService(restartResp))
	assert.Len(apiResp.ItemsRestarted, 3)
	assert.Len(apiResp.ItemsErrored, 2)
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
	})

	apiFlags := APIServiceFlags{}
	assert.NoError(apiFlags.BuildFromService(flags))
	allStructFieldsTrue(t, &apiFlags)

	newFlagsI, err := apiFlags.ToService()
	assert.NoError(err)
	newFlags, ok := newFlagsI.(evergreen.ServiceFlags)
	require.True(t, ok)
	allStructFieldsTrue(t, &newFlags)
}

func allStructFieldsTrue(t *testing.T, s any) {
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
			"EVG": {
				Fields: map[string]string{
					"customfield_12345": "{{.Something}}",
					"customfield_12346": "{{.SomethingElse}}",
				},
				Components: []string{"component1", "component0"},
			},
			"GVE": {
				Fields: map[string]string{
					"customfield_54321": "{{.SomethingElser}}",
					"customfield_54322": "{{.SomethingEvenElser}}",
				},
				Labels: []string{"label1", "label0"},
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

func TestAPIOverride(t *testing.T) {
	t.Run("MarshalJSON", func(t *testing.T) {
		for name, testCase := range map[string]struct {
			input    APIOverride
			expected string
		}{
			"StringValue": {
				input: APIOverride{
					Value: "I'm a string",
				},
				expected: `{"section_id":null,"field":null,"value":"I'm a string"}`,
			},
			"IntValue": {
				input: APIOverride{
					Value: 42,
				},
				expected: `{"section_id":null,"field":null,"value":42}`,
			},
			"DateValue": {
				input: APIOverride{
					Value: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				},
				expected: `{"section_id":null,"field":null,"value":{"$date":"2009-11-10T23:00:00Z"}}`,
			},
		} {
			t.Run(name, func(t *testing.T) {
				overrideJSON, err := json.Marshal(testCase.input)
				assert.NoError(t, err)
				assert.Equal(t, testCase.expected, string(overrideJSON))
			})
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		for name, testCase := range map[string]struct {
			input    string
			expected APIOverride
		}{
			"StringValue": {
				input: `{"section_id":null,"field":null,"value":"I'm a string"}`,
				expected: APIOverride{
					Value: "I'm a string",
				},
			},
			"IntValue": {
				input: `{"section_id":null,"field":null,"value":42}`,
				expected: APIOverride{
					Value: int32(42),
				},
			},
			"DateValue": {
				input: `{"section_id":null,"field":null,"value":{"$date":"2009-11-10T23:00:00Z"}}`,
				expected: APIOverride{
					Value: primitive.NewDateTimeFromTime(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
				},
			},
		} {
			t.Run(name, func(t *testing.T) {
				var override APIOverride
				assert.NoError(t, json.Unmarshal([]byte(testCase.input), &override))
				assert.Equal(t, testCase.expected, override)
			})
		}
	})

}

func TestAPIS3UploadCostConfig(t *testing.T) {
	t.Run("BuildFromService", func(t *testing.T) {
		t.Run("WithZeroValue", func(t *testing.T) {
			svc := evergreen.S3UploadCostConfig{
				UploadCostDiscount: 0.0,
			}
			api := APIS3UploadCostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.0, api.UploadCostDiscount)
		})

		t.Run("WithNonZeroValue", func(t *testing.T) {
			svc := evergreen.S3UploadCostConfig{
				UploadCostDiscount: 0.25,
			}
			api := APIS3UploadCostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.25, api.UploadCostDiscount)
		})
	})

	t.Run("ToService", func(t *testing.T) {
		t.Run("RoundTrip", func(t *testing.T) {
			api := APIS3UploadCostConfig{
				UploadCostDiscount: 0.3,
			}
			svcInterface, err := api.ToService()
			assert.NoError(t, err)
			svc := svcInterface.(evergreen.S3UploadCostConfig)
			assert.Equal(t, 0.3, svc.UploadCostDiscount)
		})
	})
}

func TestAPIS3StorageCostConfig(t *testing.T) {
	t.Run("BuildFromService", func(t *testing.T) {
		t.Run("WithMixedValues", func(t *testing.T) {
			svc := evergreen.S3StorageCostConfig{
				StandardStorageCostDiscount: 0.1,
				IAStorageCostDiscount:       0.0,
			}
			api := APIS3StorageCostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.1, api.StandardStorageCostDiscount)
			assert.Equal(t, 0.0, api.IAStorageCostDiscount)
		})

		t.Run("WithAllValues", func(t *testing.T) {
			svc := evergreen.S3StorageCostConfig{
				StandardStorageCostDiscount: 0.2,
				IAStorageCostDiscount:       0.4,
			}
			api := APIS3StorageCostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.2, api.StandardStorageCostDiscount)
			assert.Equal(t, 0.4, api.IAStorageCostDiscount)
		})
	})

	t.Run("ToService", func(t *testing.T) {
		t.Run("RoundTrip", func(t *testing.T) {
			api := APIS3StorageCostConfig{
				StandardStorageCostDiscount: 0.25,
				IAStorageCostDiscount:       0.35,
			}
			svcInterface, err := api.ToService()
			assert.NoError(t, err)
			svc := svcInterface.(evergreen.S3StorageCostConfig)
			assert.Equal(t, 0.25, svc.StandardStorageCostDiscount)
			assert.Equal(t, 0.35, svc.IAStorageCostDiscount)
		})
	})
}

func TestAPIS3CostConfig(t *testing.T) {
	t.Run("BuildFromService", func(t *testing.T) {
		t.Run("WithEmptyConfig", func(t *testing.T) {
			svc := evergreen.S3CostConfig{}
			api := APIS3CostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.0, api.Upload.UploadCostDiscount)
			assert.Equal(t, 0.0, api.Storage.StandardStorageCostDiscount)
			assert.Equal(t, 0.0, api.Storage.IAStorageCostDiscount)
		})

		t.Run("WithUploadOnly", func(t *testing.T) {
			svc := evergreen.S3CostConfig{
				Upload: evergreen.S3UploadCostConfig{
					UploadCostDiscount: 0.1,
				},
			}
			api := APIS3CostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.1, api.Upload.UploadCostDiscount)
			assert.Equal(t, 0.0, api.Storage.StandardStorageCostDiscount)
		})

		t.Run("WithStorageOnly", func(t *testing.T) {
			svc := evergreen.S3CostConfig{
				Storage: evergreen.S3StorageCostConfig{
					StandardStorageCostDiscount: 0.2,
				},
			}
			api := APIS3CostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.0, api.Upload.UploadCostDiscount)
			assert.Equal(t, 0.2, api.Storage.StandardStorageCostDiscount)
		})

		t.Run("WithAllFields", func(t *testing.T) {
			svc := evergreen.S3CostConfig{
				Upload: evergreen.S3UploadCostConfig{
					UploadCostDiscount: 0.1,
				},
				Storage: evergreen.S3StorageCostConfig{
					StandardStorageCostDiscount: 0.2,
					IAStorageCostDiscount:       0.3,
				},
			}
			api := APIS3CostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			assert.Equal(t, 0.1, api.Upload.UploadCostDiscount)
			assert.Equal(t, 0.2, api.Storage.StandardStorageCostDiscount)
			assert.Equal(t, 0.3, api.Storage.IAStorageCostDiscount)
		})
	})

	t.Run("ToService", func(t *testing.T) {
		t.Run("RoundTrip", func(t *testing.T) {
			api := APIS3CostConfig{
				Upload: APIS3UploadCostConfig{
					UploadCostDiscount: 0.1,
				},
				Storage: APIS3StorageCostConfig{
					StandardStorageCostDiscount: 0.2,
					IAStorageCostDiscount:       0.3,
				},
			}
			svcInterface, err := api.ToService()
			assert.NoError(t, err)
			svc := svcInterface.(evergreen.S3CostConfig)
			assert.Equal(t, 0.1, svc.Upload.UploadCostDiscount)
			assert.Equal(t, 0.2, svc.Storage.StandardStorageCostDiscount)
			assert.Equal(t, 0.3, svc.Storage.IAStorageCostDiscount)
		})
	})
}

func TestAPICostConfigWithS3Cost(t *testing.T) {
	t.Run("BuildFromService", func(t *testing.T) {
		t.Run("WithS3Cost", func(t *testing.T) {
			formula := 0.5
			savingsPlan := 0.3
			onDemand := 0.2
			upload := 0.1
			standard := 0.15
			infrequent := 0.25

			svc := evergreen.CostConfig{
				FinanceFormula:      formula,
				SavingsPlanDiscount: savingsPlan,
				OnDemandDiscount:    onDemand,
				S3Cost: evergreen.S3CostConfig{
					Upload: evergreen.S3UploadCostConfig{
						UploadCostDiscount: upload,
					},
					Storage: evergreen.S3StorageCostConfig{
						StandardStorageCostDiscount: standard,
						IAStorageCostDiscount:       infrequent,
					},
				},
			}

			api := APICostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			require.NotNil(t, api.FinanceFormula)
			assert.Equal(t, 0.5, *api.FinanceFormula)
			require.NotNil(t, api.SavingsPlanDiscount)
			assert.Equal(t, 0.3, *api.SavingsPlanDiscount)
			require.NotNil(t, api.OnDemandDiscount)
			assert.Equal(t, 0.2, *api.OnDemandDiscount)
			require.NotNil(t, api.S3Cost)
			assert.Equal(t, 0.1, api.S3Cost.Upload.UploadCostDiscount)
			assert.Equal(t, 0.15, api.S3Cost.Storage.StandardStorageCostDiscount)
			assert.Equal(t, 0.25, api.S3Cost.Storage.IAStorageCostDiscount)
		})

		t.Run("WithPointer", func(t *testing.T) {
			formula := 0.5
			upload := 0.1
			svc := &evergreen.CostConfig{
				FinanceFormula: formula,
				S3Cost: evergreen.S3CostConfig{
					Upload: evergreen.S3UploadCostConfig{
						UploadCostDiscount: upload,
					},
				},
			}

			api := APICostConfig{}
			assert.NoError(t, api.BuildFromService(svc))
			require.NotNil(t, api.FinanceFormula)
			assert.Equal(t, 0.5, *api.FinanceFormula)
			require.NotNil(t, api.S3Cost)
			assert.Equal(t, 0.1, api.S3Cost.Upload.UploadCostDiscount)
		})
	})

	t.Run("ToService", func(t *testing.T) {
		t.Run("RoundTripWithS3Cost", func(t *testing.T) {
			formula := 0.5
			savingsPlan := 0.3
			onDemand := 0.2
			upload := 0.1
			standard := 0.15

			api := APICostConfig{
				FinanceFormula:      &formula,
				SavingsPlanDiscount: &savingsPlan,
				OnDemandDiscount:    &onDemand,
				S3Cost: &APIS3CostConfig{
					Upload: APIS3UploadCostConfig{
						UploadCostDiscount: upload,
					},
					Storage: APIS3StorageCostConfig{
						StandardStorageCostDiscount: standard,
					},
				},
			}

			svcInterface, err := api.ToService()
			assert.NoError(t, err)
			svc := svcInterface.(evergreen.CostConfig)
			assert.Equal(t, 0.5, svc.FinanceFormula)
			assert.Equal(t, 0.3, svc.SavingsPlanDiscount)
			assert.Equal(t, 0.2, svc.OnDemandDiscount)
			assert.Equal(t, 0.1, svc.S3Cost.Upload.UploadCostDiscount)
			assert.Equal(t, 0.15, svc.S3Cost.Storage.StandardStorageCostDiscount)
		})

		t.Run("WithNilS3Cost", func(t *testing.T) {
			formula := 0.5
			api := APICostConfig{
				FinanceFormula: &formula,
				S3Cost:         nil,
			}

			svcInterface, err := api.ToService()
			assert.NoError(t, err)
			svc := svcInterface.(evergreen.CostConfig)
			assert.Equal(t, 0.5, svc.FinanceFormula)
			assert.Equal(t, 0.0, svc.S3Cost.Upload.UploadCostDiscount)
			assert.Equal(t, 0.0, svc.S3Cost.Storage.StandardStorageCostDiscount)
		})
	})
}
