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
	"github.com/evergreen-ci/utility"
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
	s.Require().NoError(b.Insert())
	s.Require().NoError(v.Insert())
	s.Require().NoError(testTask1.Insert())
	s.Require().NoError(testTask2.Insert())
	s.Require().NoError(testTask3.Insert())
	s.Require().NoError(p.Insert())
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	u := &user.DBUser{Id: "user"}
	testSettings := testutil.MockConfig()
	// convert the DB model to an API model
	restSettings := restModel.NewConfigModel()
	err := restSettings.BuildFromService(testSettings)
	s.Require().NoError(err)

	// try to set the DB model with this API model
	oldSettings, err := evergreen.GetConfig()
	s.NoError(err)
	_, err = SetEvergreenSettings(restSettings, oldSettings, u, true)
	s.Require().NoError(err)

	// read the settings and spot check values
	settingsFromConnector, err := evergreen.GetConfig()
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
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settingsFromConnector.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.LDAP.URL, settingsFromConnector.AuthConfig.LDAP.URL)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settingsFromConnector.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.Equal(testSettings.AuthConfig.Multi.ReadWrite[0], settingsFromConnector.AuthConfig.Multi.ReadWrite[0])
	s.EqualValues(testSettings.HostJasper.URL, settingsFromConnector.HostJasper.URL)
	s.EqualValues(testSettings.HostInit.HostThrottle, settingsFromConnector.HostInit.HostThrottle)
	s.EqualValues(testSettings.HostInit.ProvisioningThrottle, settingsFromConnector.HostInit.ProvisioningThrottle)
	s.EqualValues(testSettings.HostInit.CloudStatusBatchSize, settingsFromConnector.HostInit.CloudStatusBatchSize)
	s.EqualValues(testSettings.HostInit.MaxTotalDynamicHosts, settingsFromConnector.HostInit.MaxTotalDynamicHosts)
	s.EqualValues(testSettings.HostInit.S3BaseURL, settingsFromConnector.HostInit.S3BaseURL)
	s.EqualValues(testSettings.PodLifecycle.S3BaseURL, settingsFromConnector.PodLifecycle.S3BaseURL)
	s.EqualValues(testSettings.PodLifecycle.MaxParallelPodRequests, settingsFromConnector.PodLifecycle.MaxParallelPodRequests)
	s.EqualValues(testSettings.PodLifecycle.MaxPodDefinitionCleanupRate, settingsFromConnector.PodLifecycle.MaxPodDefinitionCleanupRate)
	s.EqualValues(testSettings.PodLifecycle.MaxSecretCleanupRate, settingsFromConnector.PodLifecycle.MaxSecretCleanupRate)
	s.EqualValues(testSettings.Jira.BasicAuthConfig.Username, settingsFromConnector.Jira.BasicAuthConfig.Username)

	s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)

	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, settingsFromConnector.LoggerConfig.Buffer.IncomingBufferFactor)
	s.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, settingsFromConnector.LoggerConfig.Buffer.UseAsync)
	s.EqualValues(testSettings.Notify.SES.SenderAddress, settingsFromConnector.Notify.SES.SenderAddress)
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.Equal(testSettings.Providers.AWS.ParserProject.Key, settingsFromConnector.Providers.AWS.ParserProject.Key)
	s.Equal(testSettings.Providers.AWS.ParserProject.Secret, settingsFromConnector.Providers.AWS.ParserProject.Secret)
	s.Equal(testSettings.Providers.AWS.ParserProject.Bucket, settingsFromConnector.Providers.AWS.ParserProject.Bucket)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodInitDisabled, settingsFromConnector.ServiceFlags.PodInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodAllocatorDisabled, settingsFromConnector.ServiceFlags.PodAllocatorDisabled)
	s.EqualValues(testSettings.ServiceFlags.UnrecognizedPodCleanupDisabled, settingsFromConnector.ServiceFlags.UnrecognizedPodCleanupDisabled)
	s.EqualValues(testSettings.ServiceFlags.S3BinaryDownloadsDisabled, settingsFromConnector.ServiceFlags.S3BinaryDownloadsDisabled)
	s.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, settingsFromConnector.ServiceFlags.CloudCleanupDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, settingsFromConnector.Splunk.SplunkConnectionInfo.Channel)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)
	s.EqualValues(testSettings.Tracer.Enabled, settingsFromConnector.Tracer.Enabled)
	s.EqualValues(testSettings.Tracer.CollectorEndpoint, settingsFromConnector.Tracer.CollectorEndpoint)

	// spot check events in the event log
	events, err := event.FindAdmin(event.RecentAdminEvents(1000))
	s.NoError(err)
	foundNotifyEvent := false
	foundFlagsEvent := false
	foundProvidersEvent := false
	foundRootEvent := false
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
			s.Require().True(len(v.AWS.EC2Keys) > 0)
			s.Equal(testSettings.Providers.AWS.EC2Keys[0].Key, v.AWS.EC2Keys[0].Key)
			s.Equal(testSettings.Providers.GCE.ClientEmail, v.GCE.ClientEmail)
		case *evergreen.Settings:
			foundRootEvent = true
			s.Equal(testSettings.ClientBinariesDir, v.ClientBinariesDir)
			s.Equal(testSettings.Credentials, v.Credentials)
		case *evergreen.UIConfig:
			foundUiEvent = true
			s.Equal(testSettings.Ui.Url, v.Url)
			s.Equal(testSettings.Ui.CacheTemplates, v.CacheTemplates)
		}
	}
	s.True(foundNotifyEvent)
	s.True(foundFlagsEvent)
	s.True(foundProvidersEvent)
	s.True(foundRootEvent)
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
		S3BaseURL:            utility.ToStringPtr("new_s3_base_url"),
	}
	updatedSettings := restModel.APIAdminSettings{
		Banner:             &newBanner,
		Expansions:         newExpansions,
		HostInit:           &newHostInit,
		DisabledGQLQueries: newDisabledQueries,
	}
	oldSettings, err = evergreen.GetConfig()
	s.NoError(err)
	_, err = SetEvergreenSettings(&updatedSettings, oldSettings, u, true)
	s.NoError(err)
	settingsFromConnector, err = evergreen.GetConfig()
	s.Require().NoError(err)
	// new values should be set
	s.EqualValues(newBanner, settingsFromConnector.Banner)
	s.EqualValues(newDisabledQueries, settingsFromConnector.DisabledGQLQueries)
	s.EqualValues(newExpansions, settingsFromConnector.Expansions)
	s.EqualValues(newHostInit.HostThrottle, settingsFromConnector.HostInit.HostThrottle)
	s.EqualValues(newHostInit.ProvisioningThrottle, settingsFromConnector.HostInit.ProvisioningThrottle)
	s.EqualValues(newHostInit.CloudStatusBatchSize, settingsFromConnector.HostInit.CloudStatusBatchSize)
	s.EqualValues(newHostInit.MaxTotalDynamicHosts, settingsFromConnector.HostInit.MaxTotalDynamicHosts)
	s.EqualValues(utility.FromStringPtr(newHostInit.S3BaseURL), settingsFromConnector.HostInit.S3BaseURL)
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
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settingsFromConnector.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.LDAP.URL, settingsFromConnector.AuthConfig.LDAP.URL)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settingsFromConnector.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.Equal(testSettings.AuthConfig.Multi.ReadWrite[0], settingsFromConnector.AuthConfig.Multi.ReadWrite[0])
	s.EqualValues(testSettings.Jira.BasicAuthConfig.Username, settingsFromConnector.Jira.BasicAuthConfig.Username)

	s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)

	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, settingsFromConnector.LoggerConfig.Buffer.IncomingBufferFactor)
	s.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, settingsFromConnector.LoggerConfig.Buffer.UseAsync)
	s.EqualValues(testSettings.Notify.SES.SenderAddress, settingsFromConnector.Notify.SES.SenderAddress)
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.Equal(testSettings.Providers.AWS.ParserProject.Key, settingsFromConnector.Providers.AWS.ParserProject.Key)
	s.Equal(testSettings.Providers.AWS.ParserProject.Secret, settingsFromConnector.Providers.AWS.ParserProject.Secret)
	s.Equal(testSettings.Providers.AWS.ParserProject.Bucket, settingsFromConnector.Providers.AWS.ParserProject.Bucket)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodInitDisabled, settingsFromConnector.ServiceFlags.PodInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodAllocatorDisabled, settingsFromConnector.ServiceFlags.PodAllocatorDisabled)
	s.EqualValues(testSettings.ServiceFlags.UnrecognizedPodCleanupDisabled, settingsFromConnector.ServiceFlags.UnrecognizedPodCleanupDisabled)
	s.EqualValues(testSettings.ServiceFlags.S3BinaryDownloadsDisabled, settingsFromConnector.ServiceFlags.S3BinaryDownloadsDisabled)
	s.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, settingsFromConnector.ServiceFlags.CloudCleanupDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, settingsFromConnector.Splunk.SplunkConnectionInfo.Channel)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)
	s.EqualValues(testSettings.Tracer.Enabled, settingsFromConnector.Tracer.Enabled)
	s.EqualValues(testSettings.Tracer.CollectorEndpoint, settingsFromConnector.Tracer.CollectorEndpoint)
}

func (s *AdminDataSuite) TestRestart() {
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
	dryRunResp, err := RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
	s.NotZero(len(dryRunResp.ItemsRestarted))
	s.Nil(dryRunResp.ItemsErrored)

	// test that restarting tasks successfully puts a job on the queue
	opts.DryRun = false
	_, err = RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
}

func (s *AdminDataSuite) TestGetBanner() {
	u := &user.DBUser{Id: "me"}
	s.NoError(evergreen.SetBanner("banner text"))
	s.NoError(SetBannerTheme(evergreen.Important, u))
	text, theme, err := GetBanner()
	s.NoError(err)
	s.Equal("banner text", text)
	s.Equal(evergreen.Important, theme)
}
