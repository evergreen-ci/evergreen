package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/suite"
)

type AdminDataSuite struct {
	env *mock.Environment
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	suite.Run(t, s)
}

func (s *AdminDataSuite) SetupSuite() {
	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(context.Background()))
	s.NoError(db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, event.EventCollection, model.ProjectRefCollection))
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &model.Version{
		Id:     b.Version,
		Status: evergreen.VersionStarted,
	}
	testTask1 := &task.Task{
		Id:        "taskToRestart",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Type: evergreen.CommandTypeTest,
		},
	}
	testTask2 := &task.Task{
		Id:        "taskThatSucceeded",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 12, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskSucceeded,
	}
	testTask3 := &task.Task{
		Id:        "taskOutsideOfTimeRange",
		Activated: false,
		BuildId:   b.Id,
		Execution: 1,
		Project:   "sample",
		StartTime: time.Date(2017, time.June, 11, 12, 0, 0, 0, time.Local),
		Status:    evergreen.TaskFailed,
	}
	p := &model.ProjectRef{
		Id: "sample",
	}
	s.Require().NoError(b.Insert(s.T().Context()))
	s.Require().NoError(v.Insert(s.T().Context()))
	s.Require().NoError(testTask1.Insert(s.T().Context()))
	s.Require().NoError(testTask2.Insert(s.T().Context()))
	s.Require().NoError(testTask3.Insert(s.T().Context()))
	s.Require().NoError(p.Insert(s.T().Context()))
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	u := &user.DBUser{Id: "user"}
	testSettings := testutil.MockConfig()
	// convert the DB model to an API model
	restSettings := restModel.NewConfigModel()
	err := restSettings.BuildFromService(testSettings)
	s.Require().NoError(err)

	// try to set the DB model with this API model
	oldSettings, err := evergreen.GetConfig(ctx)
	s.NoError(err)
	_, err = SetEvergreenSettings(ctx, restSettings, oldSettings, u, true)
	s.Require().NoError(err)

	// read the settings and spot check values
	settingsFromConnector, err := evergreen.GetConfig(ctx)
	s.Require().NoError(err)
	s.EqualValues(testSettings.DisabledGQLQueries, settingsFromConnector.DisabledGQLQueries)
	s.EqualValues(testSettings.Banner, settingsFromConnector.Banner)
	s.EqualValues(testSettings.ServiceFlags, settingsFromConnector.ServiceFlags)
	s.EqualValues(evergreen.Important, testSettings.BannerTheme)
	s.EqualValues(testSettings.Amboy.Name, settingsFromConnector.Amboy.Name)
	s.EqualValues(testSettings.Amboy.LocalStorage, settingsFromConnector.Amboy.LocalStorage)
	s.EqualValues(testSettings.Amboy.GroupDefaultWorkers, settingsFromConnector.Amboy.GroupDefaultWorkers)
	s.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, settingsFromConnector.Amboy.GroupBackgroundCreateFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, settingsFromConnector.Amboy.GroupPruneFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupTTLMinutes, settingsFromConnector.Amboy.GroupTTLMinutes)
	s.EqualValues(testSettings.Amboy.LockTimeoutMinutes, settingsFromConnector.Amboy.LockTimeoutMinutes)
	s.EqualValues(testSettings.Amboy.SampleSize, settingsFromConnector.Amboy.SampleSize)
	s.EqualValues(testSettings.Amboy.Retry, settingsFromConnector.Amboy.Retry)
	s.EqualValues(testSettings.Amboy.NamedQueues, settingsFromConnector.Amboy.NamedQueues)
	s.EqualValues(testSettings.AmboyDB.URL, settingsFromConnector.AmboyDB.URL)
	s.EqualValues(testSettings.AmboyDB.Database, settingsFromConnector.AmboyDB.Database)
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.Api.URL, settingsFromConnector.Api.URL)
	s.EqualValues(testSettings.Api.CorpURL, settingsFromConnector.Api.CorpURL)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settingsFromConnector.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settingsFromConnector.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.Equal(testSettings.AuthConfig.Multi.ReadWrite[0], settingsFromConnector.AuthConfig.Multi.ReadWrite[0])
	s.EqualValues(testSettings.AuthConfig.Kanopy.Issuer, settingsFromConnector.AuthConfig.Kanopy.Issuer)
	s.Equal(testSettings.Buckets.Credentials.Key, settingsFromConnector.Buckets.Credentials.Key)
	s.Equal(testSettings.Buckets.Credentials.Secret, settingsFromConnector.Buckets.Credentials.Secret)
	s.Equal(testSettings.FWS.URL, settingsFromConnector.FWS.URL)
	s.EqualValues(testSettings.HostJasper.URL, settingsFromConnector.HostJasper.URL)
	s.EqualValues(testSettings.HostInit.HostThrottle, settingsFromConnector.HostInit.HostThrottle)
	s.EqualValues(testSettings.HostInit.ProvisioningThrottle, settingsFromConnector.HostInit.ProvisioningThrottle)
	s.EqualValues(testSettings.HostInit.CloudStatusBatchSize, settingsFromConnector.HostInit.CloudStatusBatchSize)
	s.EqualValues(testSettings.HostInit.MaxTotalDynamicHosts, settingsFromConnector.HostInit.MaxTotalDynamicHosts)
	s.EqualValues(testSettings.Jira.PersonalAccessToken, settingsFromConnector.Jira.PersonalAccessToken)

	s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)

	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, settingsFromConnector.LoggerConfig.Buffer.IncomingBufferFactor)
	s.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, settingsFromConnector.LoggerConfig.Buffer.UseAsync)
	s.EqualValues(testSettings.Notify.SES.SenderAddress, settingsFromConnector.Notify.SES.SenderAddress)
	s.EqualValues(testSettings.Overrides.Overrides[0].SectionID, settingsFromConnector.Overrides.Overrides[0].SectionID)
	s.EqualValues(testSettings.Overrides.Overrides[0].Field, settingsFromConnector.Overrides.Overrides[0].Field)
	s.EqualValues(testSettings.Overrides.Overrides[0].Value, settingsFromConnector.Overrides.Overrides[0].Value)
	s.Equal(testSettings.ParameterStore.Prefix, settingsFromConnector.ParameterStore.Prefix)
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.Equal(testSettings.Providers.AWS.ParserProject.Key, settingsFromConnector.Providers.AWS.ParserProject.Key)
	s.Equal(testSettings.Providers.AWS.ParserProject.Secret, settingsFromConnector.Providers.AWS.ParserProject.Secret)
	s.Equal(testSettings.Providers.AWS.ParserProject.Bucket, settingsFromConnector.Providers.AWS.ParserProject.Bucket)
	s.Equal(testSettings.Providers.AWS.ParserProject.Prefix, settingsFromConnector.Providers.AWS.ParserProject.Prefix)
	s.Equal(testSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix, settingsFromConnector.Providers.AWS.ParserProject.GeneratedJSONPrefix)
	s.Equal(testSettings.Providers.AWS.PersistentDNS.HostedZoneID, settingsFromConnector.Providers.AWS.PersistentDNS.HostedZoneID)
	s.Equal(testSettings.Providers.AWS.PersistentDNS.Domain, settingsFromConnector.Providers.AWS.PersistentDNS.Domain)
	s.Require().Len(testSettings.Providers.AWS.AccountRoles, len(settingsFromConnector.Providers.AWS.AccountRoles))
	for i := range testSettings.Providers.AWS.AccountRoles {
		s.Equal(testSettings.Providers.AWS.AccountRoles[i], settingsFromConnector.Providers.AWS.AccountRoles[i])
	}
	s.Equal(testSettings.Providers.AWS.IPAMPoolID, settingsFromConnector.Providers.AWS.IPAMPoolID)
	s.Equal(testSettings.Providers.AWS.ElasticIPUsageRate, settingsFromConnector.Providers.AWS.ElasticIPUsageRate)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.ReleaseMode.DistroMaxHostsFactor, settingsFromConnector.ReleaseMode.DistroMaxHostsFactor)
	s.EqualValues(testSettings.ReleaseMode.TargetTimeSecondsOverride, settingsFromConnector.ReleaseMode.TargetTimeSecondsOverride)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.LargeParserProjectsDisabled, settingsFromConnector.ServiceFlags.LargeParserProjectsDisabled)
	s.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, settingsFromConnector.ServiceFlags.CloudCleanupDisabled)
	s.EqualValues(testSettings.ServiceFlags.SleepScheduleDisabled, settingsFromConnector.ServiceFlags.SleepScheduleDisabled)
	s.EqualValues(testSettings.ServiceFlags.StaticAPIKeysDisabled, settingsFromConnector.ServiceFlags.StaticAPIKeysDisabled)
	s.EqualValues(testSettings.ServiceFlags.JWTTokenForCLIDisabled, settingsFromConnector.ServiceFlags.JWTTokenForCLIDisabled)
	s.EqualValues(testSettings.ServiceFlags.SystemFailedTaskRestartDisabled, settingsFromConnector.ServiceFlags.SystemFailedTaskRestartDisabled)
	s.EqualValues(testSettings.ServiceFlags.CPUDegradedModeDisabled, settingsFromConnector.ServiceFlags.CPUDegradedModeDisabled)
	s.EqualValues(testSettings.ServiceFlags.ElasticIPsDisabled, settingsFromConnector.ServiceFlags.ElasticIPsDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.ElementsMatch(testSettings.SleepSchedule.PermanentlyExemptHosts, settingsFromConnector.SleepSchedule.PermanentlyExemptHosts)
	s.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, settingsFromConnector.Splunk.SplunkConnectionInfo.Channel)
	s.EqualValues(testSettings.SSH.SpawnHostKey.Name, settingsFromConnector.SSH.SpawnHostKey.Name)
	s.EqualValues(testSettings.SSH.SpawnHostKey.SecretARN, settingsFromConnector.SSH.SpawnHostKey.SecretARN)
	s.EqualValues(testSettings.SSH.TaskHostKey.Name, settingsFromConnector.SSH.TaskHostKey.Name)
	s.EqualValues(testSettings.SSH.TaskHostKey.SecretARN, settingsFromConnector.SSH.TaskHostKey.SecretARN)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)
	s.EqualValues(testSettings.Ui.StagingEnvironment, settingsFromConnector.Ui.StagingEnvironment)
	s.EqualValues(testSettings.TestSelection.URL, settingsFromConnector.TestSelection.URL)
	s.EqualValues(testSettings.Tracer.Enabled, settingsFromConnector.Tracer.Enabled)
	s.EqualValues(testSettings.Tracer.CollectorEndpoint, settingsFromConnector.Tracer.CollectorEndpoint)
	s.EqualValues(testSettings.Tracer.CollectorInternalEndpoint, settingsFromConnector.Tracer.CollectorInternalEndpoint)

	// Check that secrets are stored in the parameter manager.
	paramMgr := evergreen.GetEnvironment().ParameterManager()
	s.Require().NotNil(paramMgr)
	secret, err := paramMgr.Get(ctx, "Settings/AuthConfig/OktaConfig/ClientSecret")
	s.NoError(err)
	s.Equal(testSettings.AuthConfig.Okta.ClientSecret, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/BucketsConfig/S3Credentials/Key")
	s.NoError(err)
	s.Equal(testSettings.Buckets.Credentials.Key, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/BucketsConfig/S3Credentials/Secret")
	s.NoError(err)
	s.Equal(testSettings.Buckets.Credentials.Secret, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/JiraConfig/PersonalAccessToken")
	s.NoError(err)
	s.Equal(testSettings.Jira.PersonalAccessToken, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/CloudProviders/AWSConfig/0/EC2Key/Key")
	s.NoError(err)
	s.Equal(testSettings.Providers.AWS.EC2Keys[0].Key, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/CloudProviders/AWSConfig/0/EC2Key/Secret")
	s.NoError(err)
	s.Equal(testSettings.Providers.AWS.EC2Keys[0].Secret, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/SlackConfig/Token")
	s.NoError(err)
	s.Equal(testSettings.Slack.Token, secret[0].Value)
	secret, err = paramMgr.Get(ctx, "Settings/Expansions/k2")
	s.NoError(err)
	s.Equal(testSettings.Expansions["k2"], secret[0].Value)

	// Read the settings from parameter manager and spot check secrets.
	paramSettings, err := evergreen.GetConfig(ctx)
	s.Require().NoError(err)
	s.Equal(testSettings.AuthConfig.Okta.ClientID, paramSettings.AuthConfig.Okta.ClientID)
	s.Equal(testSettings.Buckets.Credentials.Key, paramSettings.Buckets.Credentials.Key)
	s.Equal(testSettings.Buckets.Credentials.Secret, paramSettings.Buckets.Credentials.Secret)
	s.Equal(testSettings.Jira.PersonalAccessToken, paramSettings.Jira.PersonalAccessToken)
	s.Equal(testSettings.Providers.AWS.EC2Keys[0].Key, paramSettings.Providers.AWS.EC2Keys[0].Key)
	s.Equal(testSettings.Providers.AWS.EC2Keys[0].Secret, paramSettings.Providers.AWS.EC2Keys[0].Secret)
	s.Equal(testSettings.Slack.Token, paramSettings.Slack.Token)
	s.Equal(testSettings.Expansions["k2"], paramSettings.Expansions["k2"])

	// spot check events in the event log
	events, err := event.FindAdmin(s.T().Context(), event.RecentAdminEvents(1000))
	s.NoError(err)
	foundNotifyEvent := false
	foundFlagsEvent := false
	foundProvidersEvent := false
	foundUiEvent := false
	for _, evt := range events {
		s.Equal(event.EventTypeValueChanged, evt.EventType)
		data := evt.Data.(*event.AdminEventData)
		s.Equal(u.Id, data.User)
		switch v := data.Changes.After.(type) {
		case *evergreen.NotifyConfig:
			foundNotifyEvent = true
			s.Equal(testSettings.Notify.SES.SenderAddress, v.SES.SenderAddress)
		case *evergreen.ServiceFlags:
			foundFlagsEvent = true
			s.Equal(testSettings.ServiceFlags.RepotrackerDisabled, v.RepotrackerDisabled)
		case *evergreen.CloudProviders:
			foundProvidersEvent = true
			s.Require().NotEmpty(v.AWS.EC2Keys)
			// Verify that the key is a timestamp
			layout := "2006-01-02 15:04:05 Z0700 MST"
			_, err := time.Parse(layout, v.AWS.EC2Keys[0].Key)
			s.NoError(err)
		case *evergreen.UIConfig:
			foundUiEvent = true
			s.Equal(testSettings.Ui.Url, v.Url)
			s.Equal(testSettings.Ui.CacheTemplates, v.CacheTemplates)
		}
	}
	s.True(foundNotifyEvent)
	s.True(foundFlagsEvent)
	s.True(foundProvidersEvent)
	s.True(foundUiEvent)

	// test that updating the model with nil values does not change them
	newBanner := "new banner"
	newDisabledQueries := []string{"DisabledGQLOperationName"}
	newExpansions := map[string]string{"newkey": "newval"}
	newHostInit := restModel.APIHostInitConfig{
		HostThrottle:         64,
		CloudStatusBatchSize: 1,
		ProvisioningThrottle: 200,
		MaxTotalDynamicHosts: 1000,
	}
	updatedSettings := restModel.APIAdminSettings{
		Banner:             &newBanner,
		Expansions:         newExpansions,
		HostInit:           &newHostInit,
		DisabledGQLQueries: newDisabledQueries,
	}
	oldSettings, err = evergreen.GetConfig(ctx)
	s.NoError(err)
	_, err = SetEvergreenSettings(ctx, &updatedSettings, oldSettings, u, true)
	s.NoError(err)
	settingsFromConnector, err = evergreen.GetConfig(ctx)
	s.Require().NoError(err)
	// new values should be set
	s.EqualValues(newBanner, settingsFromConnector.Banner)
	s.EqualValues(newDisabledQueries, settingsFromConnector.DisabledGQLQueries)
	s.EqualValues(newExpansions, settingsFromConnector.Expansions)
	s.EqualValues(newHostInit.HostThrottle, settingsFromConnector.HostInit.HostThrottle)
	s.EqualValues(newHostInit.ProvisioningThrottle, settingsFromConnector.HostInit.ProvisioningThrottle)
	s.EqualValues(newHostInit.CloudStatusBatchSize, settingsFromConnector.HostInit.CloudStatusBatchSize)
	s.EqualValues(newHostInit.MaxTotalDynamicHosts, settingsFromConnector.HostInit.MaxTotalDynamicHosts)
	// old values should still be there
	s.EqualValues(testSettings.ServiceFlags, settingsFromConnector.ServiceFlags)
	s.EqualValues(evergreen.Important, testSettings.BannerTheme)
	s.EqualValues(testSettings.Amboy.Name, settingsFromConnector.Amboy.Name)
	s.EqualValues(testSettings.Amboy.LocalStorage, settingsFromConnector.Amboy.LocalStorage)
	s.EqualValues(testSettings.Amboy.GroupDefaultWorkers, settingsFromConnector.Amboy.GroupDefaultWorkers)
	s.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, settingsFromConnector.Amboy.GroupBackgroundCreateFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, settingsFromConnector.Amboy.GroupPruneFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupTTLMinutes, settingsFromConnector.Amboy.GroupTTLMinutes)
	s.EqualValues(testSettings.Amboy.LockTimeoutMinutes, settingsFromConnector.Amboy.LockTimeoutMinutes)
	s.EqualValues(testSettings.Amboy.SampleSize, settingsFromConnector.Amboy.SampleSize)
	s.EqualValues(testSettings.Amboy.Retry, settingsFromConnector.Amboy.Retry)
	s.EqualValues(testSettings.Amboy.NamedQueues, settingsFromConnector.Amboy.NamedQueues)
	s.EqualValues(testSettings.AmboyDB.URL, settingsFromConnector.AmboyDB.URL)
	s.EqualValues(testSettings.AmboyDB.Database, settingsFromConnector.AmboyDB.Database)
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.Api.URL, settingsFromConnector.Api.URL)
	s.EqualValues(testSettings.Api.CorpURL, settingsFromConnector.Api.CorpURL)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settingsFromConnector.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settingsFromConnector.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.Equal(testSettings.AuthConfig.Multi.ReadWrite[0], settingsFromConnector.AuthConfig.Multi.ReadWrite[0])
	s.EqualValues(testSettings.AuthConfig.Kanopy.Issuer, settingsFromConnector.AuthConfig.Kanopy.Issuer)
	s.Equal(testSettings.FWS.URL, settingsFromConnector.FWS.URL)
	s.EqualValues(testSettings.Jira.PersonalAccessToken, settingsFromConnector.Jira.PersonalAccessToken)
	s.Equal(testSettings.Buckets.Credentials.Key, settingsFromConnector.Buckets.Credentials.Key)
	s.Equal(testSettings.Buckets.Credentials.Secret, settingsFromConnector.Buckets.Credentials.Secret)

	s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)

	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, settingsFromConnector.LoggerConfig.Buffer.IncomingBufferFactor)
	s.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, settingsFromConnector.LoggerConfig.Buffer.UseAsync)
	s.EqualValues(testSettings.Notify.SES.SenderAddress, settingsFromConnector.Notify.SES.SenderAddress)
	s.EqualValues(testSettings.Overrides.Overrides[0].SectionID, settingsFromConnector.Overrides.Overrides[0].SectionID)
	s.EqualValues(testSettings.Overrides.Overrides[0].Field, settingsFromConnector.Overrides.Overrides[0].Field)
	s.EqualValues(testSettings.Overrides.Overrides[0].Value, settingsFromConnector.Overrides.Overrides[0].Value)
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.Equal(testSettings.Providers.AWS.ParserProject.Key, settingsFromConnector.Providers.AWS.ParserProject.Key)
	s.Equal(testSettings.Providers.AWS.ParserProject.Secret, settingsFromConnector.Providers.AWS.ParserProject.Secret)
	s.Equal(testSettings.Providers.AWS.ParserProject.Bucket, settingsFromConnector.Providers.AWS.ParserProject.Bucket)
	s.Equal(testSettings.Providers.AWS.ParserProject.Prefix, settingsFromConnector.Providers.AWS.ParserProject.Prefix)
	s.Equal(testSettings.Providers.AWS.ParserProject.GeneratedJSONPrefix, settingsFromConnector.Providers.AWS.ParserProject.GeneratedJSONPrefix)
	s.Equal(testSettings.Providers.AWS.PersistentDNS.HostedZoneID, settingsFromConnector.Providers.AWS.PersistentDNS.HostedZoneID)
	s.Equal(testSettings.Providers.AWS.PersistentDNS.Domain, settingsFromConnector.Providers.AWS.PersistentDNS.Domain)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.LargeParserProjectsDisabled, settingsFromConnector.ServiceFlags.LargeParserProjectsDisabled)
	s.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, settingsFromConnector.ServiceFlags.CloudCleanupDisabled)
	s.EqualValues(testSettings.ServiceFlags.SleepScheduleDisabled, settingsFromConnector.ServiceFlags.SleepScheduleDisabled)
	s.EqualValues(testSettings.ServiceFlags.StaticAPIKeysDisabled, settingsFromConnector.ServiceFlags.StaticAPIKeysDisabled)
	s.EqualValues(testSettings.ServiceFlags.JWTTokenForCLIDisabled, settingsFromConnector.ServiceFlags.JWTTokenForCLIDisabled)
	s.EqualValues(testSettings.ServiceFlags.SystemFailedTaskRestartDisabled, settingsFromConnector.ServiceFlags.SystemFailedTaskRestartDisabled)
	s.EqualValues(testSettings.ServiceFlags.CPUDegradedModeDisabled, settingsFromConnector.ServiceFlags.CPUDegradedModeDisabled)
	s.EqualValues(testSettings.ServiceFlags.ElasticIPsDisabled, settingsFromConnector.ServiceFlags.ElasticIPsDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.ElementsMatch(testSettings.SleepSchedule.PermanentlyExemptHosts, settingsFromConnector.SleepSchedule.PermanentlyExemptHosts)
	s.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, settingsFromConnector.Splunk.SplunkConnectionInfo.Channel)
	s.EqualValues(testSettings.SSH.SpawnHostKey.Name, settingsFromConnector.SSH.SpawnHostKey.Name)
	s.EqualValues(testSettings.SSH.SpawnHostKey.SecretARN, settingsFromConnector.SSH.SpawnHostKey.SecretARN)
	s.EqualValues(testSettings.SSH.TaskHostKey.Name, settingsFromConnector.SSH.TaskHostKey.Name)
	s.EqualValues(testSettings.SSH.TaskHostKey.SecretARN, settingsFromConnector.SSH.TaskHostKey.SecretARN)
	s.EqualValues(testSettings.TaskLimits.MaxTasksPerVersion, settingsFromConnector.TaskLimits.MaxTasksPerVersion)
	s.EqualValues(testSettings.TestSelection.URL, settingsFromConnector.TestSelection.URL)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)
	s.EqualValues(testSettings.Ui.StagingEnvironment, settingsFromConnector.Ui.StagingEnvironment)
	s.EqualValues(testSettings.Tracer.Enabled, settingsFromConnector.Tracer.Enabled)
	s.EqualValues(testSettings.Tracer.CollectorEndpoint, settingsFromConnector.Tracer.CollectorEndpoint)
	s.EqualValues(testSettings.Tracer.CollectorInternalEndpoint, settingsFromConnector.Tracer.CollectorInternalEndpoint)
}

func (s *AdminDataSuite) TestRestart() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startTime := time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	endTime := time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local)
	userName := "user"

	// test dry run
	opts := model.RestartOptions{
		DryRun:    true,
		StartTime: startTime,
		EndTime:   endTime,
		User:      userName,
	}
	dryRunResp, err := RestartFailedTasks(ctx, s.env.LocalQueue(), opts)
	s.NoError(err)
	s.NotEmpty(dryRunResp.ItemsRestarted)
	s.Nil(dryRunResp.ItemsErrored)

	// test that restarting tasks successfully puts a job on the queue
	opts.DryRun = false
	_, err = RestartFailedTasks(ctx, s.env.LocalQueue(), opts)
	s.NoError(err)
}

func (s *AdminDataSuite) TestGetBanner() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(evergreen.SetBanner(ctx, "banner text"))
	s.NoError(SetBannerTheme(ctx, string(evergreen.Important)))
	text, theme, err := GetBanner(ctx)
	s.NoError(err)
	s.Equal("banner text", text)
	s.Equal(string(evergreen.Important), theme)
}

func (s *AdminDataSuite) TestGetNecessaryServiceFlags() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{
		ServiceFlags: evergreen.ServiceFlags{
			HostInitDisabled:       true,
			RepotrackerDisabled:    true,
			TaskDispatchDisabled:   false,
			CloudCleanupDisabled:   false,
			StaticAPIKeysDisabled:  true,
			JWTTokenForCLIDisabled: false,
		},
	}
	s.NoError(evergreen.UpdateConfig(ctx, settings))

	flags, err := GetNecessaryServiceFlags(ctx)
	s.NoError(err)
	neccesaryFlags := evergreen.ServiceFlags{
		StaticAPIKeysDisabled:  true,
		JWTTokenForCLIDisabled: false,
	}
	s.Equal(neccesaryFlags, flags)
}
