package data

import (
	"context"
	"reflect"
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
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AdminDataSuite struct {
	ctx Connector
	env *mock.Environment
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &DBConnector{}
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, event.AllLogCollection), "clearing collections")
	defer func() {
		assert.NoError(t, db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, event.AllLogCollection), "clearing collections")
	}()
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
	require.NoError(t, b.Insert(), "error inserting documents")
	require.NoError(t, v.Insert(), "error inserting documents")
	require.NoError(t, testTask1.Insert(), "error inserting documents")
	require.NoError(t, testTask2.Insert(), "error inserting documents")
	require.NoError(t, testTask3.Insert(), "error inserting documents")
	require.NoError(t, p.Insert(), "error inserting documents")
	suite.Run(t, s)
}

func TestMockConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &MockConnector{}
	suite.Run(t, s)
}

func (s *AdminDataSuite) SetupSuite() {
	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(context.Background()))
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
	_, err = s.ctx.SetEvergreenSettings(restSettings, oldSettings, u, true)
	s.Require().NoError(err)

	// read the settings and spot check values
	settingsFromConnector, err := s.ctx.GetEvergreenSettings()
	s.Require().NoError(err)
	s.EqualValues(testSettings.Banner, settingsFromConnector.Banner)
	s.EqualValues(testSettings.ServiceFlags, settingsFromConnector.ServiceFlags)
	s.EqualValues(evergreen.Important, testSettings.BannerTheme)
	s.EqualValues(testSettings.Alerts.SMTP.From, settingsFromConnector.Alerts.SMTP.From)
	s.EqualValues(testSettings.Alerts.SMTP.Port, settingsFromConnector.Alerts.SMTP.Port)
	s.Equal(len(testSettings.Alerts.SMTP.AdminEmail), len(settingsFromConnector.Alerts.SMTP.AdminEmail))
	s.EqualValues(testSettings.Amboy.Name, settingsFromConnector.Amboy.Name)
	s.EqualValues(testSettings.Amboy.LocalStorage, settingsFromConnector.Amboy.LocalStorage)
	s.EqualValues(testSettings.Amboy.GroupDefaultWorkers, settingsFromConnector.Amboy.GroupDefaultWorkers)
	s.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, settingsFromConnector.Amboy.GroupBackgroundCreateFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, settingsFromConnector.Amboy.GroupPruneFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupTTLMinutes, settingsFromConnector.Amboy.GroupTTLMinutes)
	s.EqualValues(testSettings.Amboy.LockTimeoutMinutes, settingsFromConnector.Amboy.LockTimeoutMinutes)
	s.EqualValues(testSettings.Amboy.SampleSize, settingsFromConnector.Amboy.SampleSize)
	s.EqualValues(testSettings.Amboy.Retry, settingsFromConnector.Amboy.Retry)
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
	s.EqualValues(testSettings.PodInit.S3BaseURL, settingsFromConnector.PodInit.S3BaseURL)
	s.EqualValues(testSettings.Jira.BasicAuthConfig.Username, settingsFromConnector.Jira.BasicAuthConfig.Username)
	// We have to check different cases because the mock connector does not set
	// defaults for the settings.
	switch s.ctx.(type) {
	case *MockConnector:
		s.Equal(testSettings.LoggerConfig.DefaultLevel, settingsFromConnector.LoggerConfig.DefaultLevel)
	case *DBConnector:
		s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)
	default:
		s.Error(errors.New("data connector was not a DBConnector or MockConnector"))
	}
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.Notify.SMTP.From, settingsFromConnector.Notify.SMTP.From)
	s.EqualValues(testSettings.Notify.SMTP.Port, settingsFromConnector.Notify.SMTP.Port)
	s.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(settingsFromConnector.Notify.SMTP.AdminEmail))
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodInitDisabled, settingsFromConnector.ServiceFlags.PodInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.S3BinaryDownloadsDisabled, settingsFromConnector.ServiceFlags.S3BinaryDownloadsDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.EqualValues(testSettings.Splunk.Channel, settingsFromConnector.Splunk.Channel)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)

	// the tests below do not apply to the mock connector
	if reflect.TypeOf(s.ctx).String() == "*data.MockConnector" {
		return
	}

	// spot check events in the event log
	events, err := event.FindAdmin(event.RecentAdminEvents(1000))
	s.NoError(err)
	foundAlertsEvent := false
	foundFlagsEvent := false
	foundProvidersEvent := false
	foundRootEvent := false
	foundUiEvent := false
	for _, evt := range events {
		s.Equal(event.EventTypeValueChanged, evt.EventType)
		data := evt.Data.(*event.AdminEventData)
		s.Equal(u.Id, data.User)
		switch v := data.Changes.After.(type) {
		case *evergreen.AlertsConfig:
			foundAlertsEvent = true
			s.Equal(testSettings.Alerts.SMTP.From, v.SMTP.From)
			s.Equal(testSettings.Alerts.SMTP.Username, v.SMTP.Username)
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
	s.True(foundAlertsEvent)
	s.True(foundFlagsEvent)
	s.True(foundProvidersEvent)
	s.True(foundRootEvent)
	s.True(foundUiEvent)

	// test that updating the model with nil values does not change them
	newBanner := "new banner"
	newExpansions := map[string]string{"newkey": "newval"}
	newHostInit := restModel.APIHostInitConfig{
		HostThrottle:         64,
		CloudStatusBatchSize: 1,
		ProvisioningThrottle: 200,
		MaxTotalDynamicHosts: 1000,
		S3BaseURL:            utility.ToStringPtr("new_s3_base_url"),
	}
	updatedSettings := restModel.APIAdminSettings{
		Banner:     &newBanner,
		Expansions: newExpansions,
		HostInit:   &newHostInit,
	}
	oldSettings, err = evergreen.GetConfig()
	s.NoError(err)
	_, err = s.ctx.SetEvergreenSettings(&updatedSettings, oldSettings, u, true)
	s.NoError(err)
	settingsFromConnector, err = s.ctx.GetEvergreenSettings()
	s.Require().NoError(err)
	// new values should be set
	s.EqualValues(newBanner, settingsFromConnector.Banner)
	s.EqualValues(newExpansions, settingsFromConnector.Expansions)
	s.EqualValues(newHostInit.HostThrottle, settingsFromConnector.HostInit.HostThrottle)
	s.EqualValues(newHostInit.ProvisioningThrottle, settingsFromConnector.HostInit.ProvisioningThrottle)
	s.EqualValues(newHostInit.CloudStatusBatchSize, settingsFromConnector.HostInit.CloudStatusBatchSize)
	s.EqualValues(newHostInit.MaxTotalDynamicHosts, settingsFromConnector.HostInit.MaxTotalDynamicHosts)
	s.EqualValues(utility.FromStringPtr(newHostInit.S3BaseURL), settingsFromConnector.HostInit.S3BaseURL)
	// old values should still be there
	s.EqualValues(testSettings.ServiceFlags, settingsFromConnector.ServiceFlags)
	s.EqualValues(evergreen.Important, testSettings.BannerTheme)
	s.EqualValues(testSettings.Alerts.SMTP.From, settingsFromConnector.Alerts.SMTP.From)
	s.EqualValues(testSettings.Alerts.SMTP.Port, settingsFromConnector.Alerts.SMTP.Port)
	s.Equal(len(testSettings.Alerts.SMTP.AdminEmail), len(settingsFromConnector.Alerts.SMTP.AdminEmail))
	s.EqualValues(testSettings.Amboy.Name, settingsFromConnector.Amboy.Name)
	s.EqualValues(testSettings.Amboy.LocalStorage, settingsFromConnector.Amboy.LocalStorage)
	s.EqualValues(testSettings.Amboy.GroupDefaultWorkers, settingsFromConnector.Amboy.GroupDefaultWorkers)
	s.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, settingsFromConnector.Amboy.GroupBackgroundCreateFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, settingsFromConnector.Amboy.GroupPruneFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupTTLMinutes, settingsFromConnector.Amboy.GroupTTLMinutes)
	s.EqualValues(testSettings.Amboy.LockTimeoutMinutes, settingsFromConnector.Amboy.LockTimeoutMinutes)
	s.EqualValues(testSettings.Amboy.SampleSize, settingsFromConnector.Amboy.SampleSize)
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settingsFromConnector.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.LDAP.URL, settingsFromConnector.AuthConfig.LDAP.URL)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settingsFromConnector.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.Equal(testSettings.AuthConfig.Multi.ReadWrite[0], settingsFromConnector.AuthConfig.Multi.ReadWrite[0])
	s.EqualValues(testSettings.Jira.BasicAuthConfig.Username, settingsFromConnector.Jira.BasicAuthConfig.Username)
	switch s.ctx.(type) {
	case *MockConnector:
		s.Equal(testSettings.LoggerConfig.DefaultLevel, settingsFromConnector.LoggerConfig.DefaultLevel)
	case *DBConnector:
		s.Equal(level.Info.String(), settingsFromConnector.LoggerConfig.DefaultLevel)
	default:
		s.Error(errors.New("data connector was not a DBConnector or MockConnector"))
	}
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.Notify.SMTP.From, settingsFromConnector.Notify.SMTP.From)
	s.EqualValues(testSettings.Notify.SMTP.Port, settingsFromConnector.Notify.SMTP.Port)
	s.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(settingsFromConnector.Notify.SMTP.AdminEmail))
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settingsFromConnector.Providers.AWS.EC2Keys))
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.PodInitDisabled, settingsFromConnector.ServiceFlags.PodInitDisabled)
	s.EqualValues(testSettings.Slack.Level, settingsFromConnector.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settingsFromConnector.Slack.Options.Channel)
	s.EqualValues(testSettings.Splunk.Channel, settingsFromConnector.Splunk.Channel)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settingsFromConnector.Ui.HttpListenAddr)
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
	dryRunResp, err := s.ctx.RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
	s.NotZero(len(dryRunResp.ItemsRestarted))
	s.Nil(dryRunResp.ItemsErrored)

	// test that restarting tasks successfully puts a job on the queue
	opts.DryRun = false
	_, err = s.ctx.RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
}

func (s *AdminDataSuite) TestGetBanner() {
	u := &user.DBUser{Id: "me"}
	s.NoError(s.ctx.SetAdminBanner("banner text", u))
	s.NoError(s.ctx.SetBannerTheme(evergreen.Important, u))
	text, theme, err := s.ctx.GetBanner()
	s.NoError(err)
	s.Equal("banner text", text)
	s.Equal(evergreen.Important, theme)
}
