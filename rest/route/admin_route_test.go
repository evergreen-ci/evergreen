package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/send"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type AdminRouteSuite struct {
	sc data.Connector
	suite.Suite
	getHandler  MethodHandler
	postHandler MethodHandler
}

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

func TestAdminRouteSuite(t *testing.T) {
	assert := assert.New(t)
	s := new(AdminRouteSuite)
	s.sc = &data.MockConnector{}

	// test getting the route handler
	const route = "/admin"
	const version = 2
	routeManager := getAdminSettingsManager(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	s.getHandler = routeManager.Methods[0]
	s.postHandler = routeManager.Methods[1]
	assert.IsType(&adminGetHandler{}, s.getHandler.RequestHandler)
	assert.IsType(&adminPostHandler{}, s.postHandler.RequestHandler)

	// run the rest of the tests
	suite.Run(t, s)
}

func (s *AdminRouteSuite) TestAdminRoute() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "user"})
	jsonBody, err := json.Marshal(&testSettings)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))

	// test executing the POST request
	resp, err := s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)

	// test getting the settings
	s.NoError(s.getHandler.RequestHandler.ParseAndValidate(ctx, nil))
	resp, err = s.getHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)
	settingsResp, err := resp.Result[0].ToService()
	s.NoError(err)
	settings, ok := settingsResp.(evergreen.Settings)
	s.True(ok)
	s.Equal(testSettings, settings)
}

func (s *AdminRouteSuite) TestGetAuthentication() {
	superUser := user.DBUser{
		Id: "super_user",
	}
	normalUser := user.DBUser{
		Id: "normal_user",
	}
	s.sc.SetSuperUsers([]string{"super_user"})

	superCtx := context.WithValue(context.Background(), evergreen.RequestUser, &superUser)
	normalCtx := context.WithValue(context.Background(), evergreen.RequestUser, &normalUser)

	s.NoError(s.getHandler.Authenticate(superCtx, s.sc))
	s.NoError(s.getHandler.Authenticate(normalCtx, s.sc))
}

func (s *AdminRouteSuite) TestPostAuthentication() {
	superUser := user.DBUser{
		Id: "super_user",
	}
	normalUser := user.DBUser{
		Id: "normal_user",
	}
	s.sc.SetSuperUsers([]string{"super_user"})

	superCtx := context.WithValue(context.Background(), evergreen.RequestUser, &superUser)
	normalCtx := context.WithValue(context.Background(), evergreen.RequestUser, &normalUser)

	s.NoError(s.postHandler.Authenticate(superCtx, s.sc))
	s.Error(s.postHandler.Authenticate(normalCtx, s.sc))
}

func TestRestartRoute(t *testing.T) {
	assert := assert.New(t) // nolint

	ctx := context.WithValue(context.Background(), evergreen.RequestUser, &user.DBUser{Id: "userName"})
	const route = "/admin/restart"
	const version = 2

	queue := evergreen.GetEnvironment().LocalQueue()

	routeManager := getRestartRouteManager(queue)(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	handler := routeManager.Methods[0]
	startTime := time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	endTime := time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local)

	// test that invalid time range errors
	body := struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		DryRun    bool      `json:"dry_run"`
	}{endTime, startTime, false}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/restart", buffer)
	assert.NoError(err)
	assert.Error(handler.ParseAndValidate(ctx, request))

	// test a valid request
	body = struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		DryRun    bool      `json:"dry_run"`
	}{startTime, endTime, false}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest("POST", "/admin/restart", buffer)
	assert.NoError(err)
	assert.NoError(handler.ParseAndValidate(ctx, request))
	resp, err := handler.Execute(ctx, &data.MockConnector{})
	assert.NoError(err)
	assert.NotNil(resp)
	model, ok := resp.Result[0].(*restModel.RestartTasksResponse)
	assert.True(ok)
	assert.True(len(model.TasksRestarted) > 0)
	assert.Nil(model.TasksErrored)
}
