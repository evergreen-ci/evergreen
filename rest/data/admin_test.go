package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
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

type AdminDataSuite struct {
	ctx Connector
	env *mock.Environment
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &DBConnector{}
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, version.Collection), t,
		"Error clearing collections")
	b := &build.Build{
		Id:      "buildtest",
		Status:  evergreen.BuildStarted,
		Version: "abc",
	}
	v := &version.Version{
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
	testutil.HandleTestingErr(b.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(v.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask1.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask2.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(testTask3.Insert(), t, "error inserting documents")
	testutil.HandleTestingErr(p.Insert(), t, "error inserting documents")
	suite.Run(t, s)
}

func TestMockConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &MockConnector{}
	suite.Run(t, s)
}

func (s *AdminDataSuite) SetupSuite() {
	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(context.Background(), ""))
	s.Require().NoError(s.env.Local.Start(context.Background()))
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	u := &user.DBUser{Id: "user"}
	err := s.ctx.SetEvergreenSettings(&testSettings, u)
	s.NoError(err)

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
	s.EqualValues(testSettings.Api.HttpListenAddr, settingsFromConnector.Api.HttpListenAddr)
	s.EqualValues(testSettings.AuthConfig.Crowd.Username, settingsFromConnector.AuthConfig.Crowd.Username)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settingsFromConnector.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settingsFromConnector.AuthConfig.Github.ClientId)
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settingsFromConnector.AuthConfig.Github.Users))
	s.EqualValues(testSettings.HostInit.SSHTimeoutSeconds, settingsFromConnector.HostInit.SSHTimeoutSeconds)
	s.EqualValues(testSettings.Jira.Username, settingsFromConnector.Jira.Username)
	s.EqualValues(testSettings.LoggerConfig.DefaultLevel, settingsFromConnector.LoggerConfig.DefaultLevel)
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settingsFromConnector.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.NewRelic.ApplicationName, settingsFromConnector.NewRelic.ApplicationName)
	s.EqualValues(testSettings.Notify.SMTP.From, settingsFromConnector.Notify.SMTP.From)
	s.EqualValues(testSettings.Notify.SMTP.Port, settingsFromConnector.Notify.SMTP.Port)
	s.Equal(len(testSettings.Notify.SMTP.AdminEmail), len(settingsFromConnector.Notify.SMTP.AdminEmail))
	s.EqualValues(testSettings.Providers.AWS.Id, settingsFromConnector.Providers.AWS.Id)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settingsFromConnector.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.Providers.GCE.PrivateKey, settingsFromConnector.Providers.GCE.PrivateKey)
	s.EqualValues(testSettings.Providers.OpenStack.IdentityEndpoint, settingsFromConnector.Providers.OpenStack.IdentityEndpoint)
	s.EqualValues(testSettings.Providers.VSphere.Host, settingsFromConnector.Providers.VSphere.Host)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settingsFromConnector.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settingsFromConnector.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostinitDisabled, settingsFromConnector.ServiceFlags.HostinitDisabled)
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
	opts := model.RestartTaskOptions{
		DryRun:     true,
		OnlyRed:    false,
		OnlyPurple: false,
		StartTime:  startTime,
		EndTime:    endTime,
		User:       userName,
	}
	dryRunResp, err := s.ctx.RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
	s.NotZero(len(dryRunResp.TasksRestarted))
	s.Nil(dryRunResp.TasksErrored)

	// test that restarting tasks successfully puts a job on the queue
	opts.DryRun = false
	_, err = s.ctx.RestartFailedTasks(s.env.LocalQueue(), opts)
	s.NoError(err)
}
