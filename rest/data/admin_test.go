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
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, event.AllLogCollection), "Error clearing collections")
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
		Identifier: "sample",
	}

	b.Tasks = []build.TaskCache{
		{
			Id: testTask1.Id,
		},
		{
			Id: testTask2.Id,
		},
		{
			Id: testTask3.Id,
		},
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
	s.Require().NoError(s.env.Configure(context.Background(), "", nil))
	s.Require().NoError(s.env.Local.Start(context.Background()))
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	u := &user.DBUser{Id: "user"}
	testSettings := testutil.MockConfig()
	// convert the DB model to an API model
	restSettings := restModel.NewConfigModel()
	err := restSettings.BuildFromService(testSettings)
	s.NoError(err)

	// try to set the DB model with this API model
	oldSettings, err := evergreen.GetConfig()
	s.NoError(err)
	_, err = s.ctx.SetEvergreenSettings(restSettings, oldSettings, u, true)
	s.NoError(err)

	// read the settings and spot check values
	settingsFromConnector, err := s.ctx.GetEvergreenSettings()
	s.NoError(err)
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
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.LDAP.URL, settingsFromConnector.AuthConfig.LDAP.URL)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.EqualValues(testSettings.HostInit.SSHTimeoutSeconds, settingsFromConnector.HostInit.SSHTimeoutSeconds)
	s.EqualValues(testSettings.Jira.Username, settingsFromConnector.Jira.Username)
	s.EqualValues(testSettings.LoggerConfig.DefaultLevel, settingsFromConnector.LoggerConfig.DefaultLevel)
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.Notify.SMTP.From, settingsFromConnector.Notify.SMTP.From)
	s.EqualValues(testSettings.Notify.SMTP.Port, settingsFromConnector.Notify.SMTP.Port)
	s.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(settingsFromConnector.Notify.SMTP.AdminEmail))
	s.EqualValues(testSettings.Providers.AWS.EC2Key, settingsFromConnector.Providers.AWS.EC2Key)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
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
			s.Equal(testSettings.Providers.AWS.EC2Key, v.AWS.EC2Key)
			s.Equal(testSettings.Providers.GCE.ClientEmail, v.GCE.ClientEmail)
		case *evergreen.Settings:
			foundRootEvent = true
			s.Equal(testSettings.ClientBinariesDir, v.ClientBinariesDir)
			s.Equal(testSettings.Credentials, v.Credentials)
			s.Equal(testSettings.SuperUsers, v.SuperUsers)
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
	newHostinit := restModel.APIHostInitConfig{
		SSHTimeoutSeconds: 999,
	}
	updatedSettings := restModel.APIAdminSettings{
		Banner:     &newBanner,
		Expansions: newExpansions,
		HostInit:   &newHostinit,
	}
	oldSettings, err = evergreen.GetConfig()
	s.NoError(err)
	_, err = s.ctx.SetEvergreenSettings(&updatedSettings, oldSettings, u, true)
	s.NoError(err)
	settingsFromConnector, err = s.ctx.GetEvergreenSettings()
	s.NoError(err)
	// new values should be set
	s.EqualValues(newBanner, settingsFromConnector.Banner)
	s.EqualValues(newExpansions, settingsFromConnector.Expansions)
	s.EqualValues(newHostinit, settingsFromConnector.HostInit)
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
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.LDAP.URL, settingsFromConnector.AuthConfig.LDAP.URL)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.EqualValues(testSettings.Jira.Username, settingsFromConnector.Jira.Username)
	s.EqualValues(testSettings.LoggerConfig.DefaultLevel, settingsFromConnector.LoggerConfig.DefaultLevel)
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.Notify.SMTP.From, settingsFromConnector.Notify.SMTP.From)
	s.EqualValues(testSettings.Notify.SMTP.Port, settingsFromConnector.Notify.SMTP.Port)
	s.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(settingsFromConnector.Notify.SMTP.AdminEmail))
	s.EqualValues(testSettings.Providers.AWS.EC2Key, settingsFromConnector.Providers.AWS.EC2Key)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.ClientEmail, settingsFromConnector.Providers.GCE.ClientEmail)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settingsFromConnector.ServiceFlags.HostInitDisabled)
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
