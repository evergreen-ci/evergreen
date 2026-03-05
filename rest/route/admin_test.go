package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/suite"
)

type AdminRouteSuite struct {
	getHandler  gimlet.RouteHandler
	postHandler gimlet.RouteHandler
	env         evergreen.Environment

	suite.Suite
}

func TestAdminRouteSuiteWithDB(t *testing.T) {
	s := new(AdminRouteSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.env = testutil.NewEnvironment(ctx, t)
	// run the rest of the tests
	suite.Run(t, s)
}

func (s *AdminRouteSuite) SetupSuite() {
	// test getting the route handler
	s.NoError(db.ClearCollections(evergreen.ConfigCollection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, event.EventCollection, model.ProjectRefCollection), "clearing collections")
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
	s.NoError(b.Insert(s.T().Context()))
	s.NoError(v.Insert(s.T().Context()))
	s.NoError(testTask1.Insert(s.T().Context()))
	s.NoError(testTask2.Insert(s.T().Context()))
	s.NoError(testTask3.Insert(s.T().Context()))
	s.NoError(p.Insert(s.T().Context()))
	s.getHandler = makeFetchAdminSettings()
	s.postHandler = makeSetAdminSettings()
	s.IsType(&adminGetHandler{}, s.getHandler)
	s.IsType(&adminPostHandler{}, s.postHandler)
}

func (s *AdminRouteSuite) TestAdminRoute() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	s.NoError(db.Clear(distro.Collection))
	d1 := &distro.Distro{
		Id: "valid-distro",
	}
	d2 := &distro.Distro{
		Id:            "invalid-distro",
		ContainerPool: "test-pool-1",
	}
	s.NoError(d1.Insert(ctx))
	s.NoError(d2.Insert(ctx))

	testSettings := testutil.MockConfig()
	jsonBody, err := json.Marshal(testSettings)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/admin/settings", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))

	// test executing the POST request
	resp := s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	// test getting the settings
	s.NoError(s.getHandler.Parse(ctx, nil))
	resp = s.getHandler.Run(ctx)
	s.NotNil(resp)
	respm, ok := resp.Data().(restModel.Model)
	s.True(ok, "%+v", resp.Data())
	settingsResp, err := respm.ToService()
	s.NoError(err)
	settings, ok := settingsResp.(evergreen.Settings)
	s.True(ok)

	s.EqualValues(testSettings.Amboy.Name, settings.Amboy.Name)
	s.EqualValues(testSettings.Amboy.LocalStorage, settings.Amboy.LocalStorage)
	s.EqualValues(testSettings.Amboy.GroupDefaultWorkers, settings.Amboy.GroupDefaultWorkers)
	s.EqualValues(testSettings.Amboy.GroupBackgroundCreateFrequencyMinutes, settings.Amboy.GroupBackgroundCreateFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupPruneFrequencyMinutes, settings.Amboy.GroupPruneFrequencyMinutes)
	s.EqualValues(testSettings.Amboy.GroupTTLMinutes, settings.Amboy.GroupTTLMinutes)
	s.EqualValues(testSettings.Amboy.LockTimeoutMinutes, settings.Amboy.LockTimeoutMinutes)
	s.EqualValues(testSettings.Amboy.SampleSize, settings.Amboy.SampleSize)
	s.EqualValues(testSettings.Amboy.Retry, settings.Amboy.Retry)
	s.EqualValues(testSettings.Amboy.NamedQueues, settings.Amboy.NamedQueues)
	s.EqualValues(testSettings.AmboyDB.URL, settings.AmboyDB.URL)
	s.EqualValues(testSettings.AmboyDB.Database, settings.AmboyDB.Database)
	s.EqualValues(testSettings.Api.HttpListenAddr, settings.Api.HttpListenAddr)
	s.EqualValues(testSettings.Api.URL, settings.Api.URL)
	s.EqualValues(testSettings.Api.CorpURL, settings.Api.CorpURL)
	s.EqualValues(testSettings.AuthConfig.Okta.ClientID, settings.AuthConfig.Okta.ClientID)
	s.EqualValues(testSettings.AuthConfig.Naive.Users[0].Username, settings.AuthConfig.Naive.Users[0].Username)
	s.EqualValues(testSettings.AuthConfig.Github.ClientId, settings.AuthConfig.Github.ClientId)
	s.EqualValues(testSettings.AuthConfig.PreferredType, settings.AuthConfig.PreferredType)
	s.EqualValues(testSettings.AuthConfig.Multi.ReadWrite[0], settings.AuthConfig.Multi.ReadWrite[0])
	s.Equal(len(testSettings.AuthConfig.Github.Users), len(settings.AuthConfig.Github.Users))
	s.EqualValues(testSettings.Buckets.Credentials.Key, settings.Buckets.Credentials.Key)
	s.EqualValues(testSettings.Buckets.Credentials.Secret, settings.Buckets.Credentials.Secret)
	s.EqualValues(testSettings.ContainerPools.Pools[0].Distro, settings.ContainerPools.Pools[0].Distro)
	s.EqualValues(testSettings.ContainerPools.Pools[0].Id, settings.ContainerPools.Pools[0].Id)
	s.EqualValues(testSettings.ContainerPools.Pools[0].MaxContainers, settings.ContainerPools.Pools[0].MaxContainers)
	s.EqualValues(testSettings.FWS.URL, settings.FWS.URL)
	s.EqualValues(testSettings.HostJasper.URL, settings.HostJasper.URL)
	s.EqualValues(testSettings.HostInit.HostThrottle, settings.HostInit.HostThrottle)
	s.EqualValues(testSettings.Jira.PersonalAccessToken, settings.Jira.PersonalAccessToken)
	s.Equal(level.Info.String(), settings.LoggerConfig.DefaultLevel)
	s.EqualValues(testSettings.LoggerConfig.Buffer.Count, settings.LoggerConfig.Buffer.Count)
	s.EqualValues(testSettings.LoggerConfig.Buffer.UseAsync, settings.LoggerConfig.Buffer.UseAsync)
	s.EqualValues(testSettings.LoggerConfig.Buffer.IncomingBufferFactor, settings.LoggerConfig.Buffer.IncomingBufferFactor)
	s.EqualValues(testSettings.Notify.SES.SenderAddress, settings.Notify.SES.SenderAddress)
	s.EqualValues(testSettings.Overrides.Overrides[0].SectionID, settings.Overrides.Overrides[0].SectionID)
	s.EqualValues(testSettings.Overrides.Overrides[0].Field, settings.Overrides.Overrides[0].Field)
	s.EqualValues(testSettings.Overrides.Overrides[0].Value, settings.Overrides.Overrides[0].Value)
	s.EqualValues(testSettings.ParameterStore.Prefix, settings.ParameterStore.Prefix)
	s.Equal(len(testSettings.Providers.AWS.EC2Keys), len(settings.Providers.AWS.EC2Keys))
	s.EqualValues(testSettings.Providers.AWS.PersistentDNS.HostedZoneID, settings.Providers.AWS.PersistentDNS.HostedZoneID)
	s.EqualValues(testSettings.Providers.AWS.PersistentDNS.Domain, settings.Providers.AWS.PersistentDNS.Domain)
	s.Require().Len(testSettings.Providers.AWS.AccountRoles, len(settings.Providers.AWS.AccountRoles))
	for i := range testSettings.Providers.AWS.AccountRoles {
		s.Equal(testSettings.Providers.AWS.AccountRoles[i], settings.Providers.AWS.AccountRoles[i])
	}
	s.EqualValues(testSettings.Providers.AWS.IPAMPoolID, settings.Providers.AWS.IPAMPoolID)
	s.EqualValues(testSettings.Providers.AWS.ElasticIPUsageRate, settings.Providers.AWS.ElasticIPUsageRate)
	s.EqualValues(testSettings.Providers.Docker.APIVersion, settings.Providers.Docker.APIVersion)
	s.EqualValues(testSettings.RepoTracker.MaxConcurrentRequests, settings.RepoTracker.MaxConcurrentRequests)
	s.EqualValues(testSettings.Scheduler.TaskFinder, settings.Scheduler.TaskFinder)
	s.EqualValues(testSettings.ServiceFlags.HostInitDisabled, settings.ServiceFlags.HostInitDisabled)
	s.EqualValues(testSettings.ServiceFlags.LargeParserProjectsDisabled, settings.ServiceFlags.LargeParserProjectsDisabled)
	s.EqualValues(testSettings.ServiceFlags.CloudCleanupDisabled, settings.ServiceFlags.CloudCleanupDisabled)
	s.EqualValues(testSettings.ServiceFlags.SleepScheduleDisabled, settings.ServiceFlags.SleepScheduleDisabled)
	s.EqualValues(testSettings.ServiceFlags.StaticAPIKeysDisabled, settings.ServiceFlags.StaticAPIKeysDisabled)
	s.EqualValues(testSettings.ServiceFlags.JWTTokenForCLIDisabled, settings.ServiceFlags.JWTTokenForCLIDisabled)
	s.EqualValues(testSettings.ServiceFlags.SystemFailedTaskRestartDisabled, settings.ServiceFlags.SystemFailedTaskRestartDisabled)
	s.EqualValues(testSettings.ServiceFlags.CPUDegradedModeDisabled, settings.ServiceFlags.CPUDegradedModeDisabled)
	s.EqualValues(testSettings.ServiceFlags.ElasticIPsDisabled, settings.ServiceFlags.ElasticIPsDisabled)
	s.EqualValues(testSettings.Slack.Level, settings.Slack.Level)
	s.EqualValues(testSettings.Slack.Options.Channel, settings.Slack.Options.Channel)
	s.ElementsMatch(testSettings.SleepSchedule.PermanentlyExemptHosts, settings.SleepSchedule.PermanentlyExemptHosts)
	s.EqualValues(testSettings.Splunk.SplunkConnectionInfo.Channel, settings.Splunk.SplunkConnectionInfo.Channel)
	s.EqualValues(testSettings.SSH.SpawnHostKey.Name, settings.SSH.SpawnHostKey.Name)
	s.EqualValues(testSettings.SSH.SpawnHostKey.SecretARN, settings.SSH.SpawnHostKey.SecretARN)
	s.EqualValues(testSettings.SSH.TaskHostKey.Name, settings.SSH.TaskHostKey.Name)
	s.EqualValues(testSettings.SSH.TaskHostKey.SecretARN, settings.SSH.TaskHostKey.SecretARN)
	s.EqualValues(testSettings.TaskLimits.MaxTasksPerVersion, settings.TaskLimits.MaxTasksPerVersion)
	s.EqualValues(testSettings.TestSelection.URL, settings.TestSelection.URL)
	s.EqualValues(testSettings.Ui.HttpListenAddr, settings.Ui.HttpListenAddr)
	s.EqualValues(testSettings.Ui.StagingEnvironment, settings.Ui.StagingEnvironment)

	// test that invalid input errors
	badSettingsOne := testutil.MockConfig()
	badSettingsOne.ConfigDir = ""
	badSettingsOne.Ui.CsrfKey = "12345"
	jsonBody, err = json.Marshal(badSettingsOne)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/admin", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.NotNil(resp)
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "config directory must not be empty")
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "CSRF key must be 32 characters long")

	// test that invalid container pools errors
	badSettingsTwo := testutil.MockConfig()
	badSettingsTwo.ContainerPools.Pools = []evergreen.ContainerPool{
		{
			Distro:        "valid-distro",
			Id:            "test-pool-1",
			MaxContainers: 100,
		},
		{
			Distro:        "invalid-distro",
			Id:            "test-pool-2",
			MaxContainers: 100,
		},
		{
			Distro:        "missing-distro",
			Id:            "test-pool-3",
			MaxContainers: 100,
		},
	}
	jsonBody, err = json.Marshal(badSettingsTwo)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/admin", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.Parse(ctx, request))
	resp = s.postHandler.Run(ctx)
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "container pool 'test-pool-2' has invalid distro 'invalid-distro'")
	s.Contains(resp.Data().(gimlet.ErrorResponse).Message, "distro not found for container pool 'test-pool-3'")
	s.NotNil(resp)
}

func (s *AdminRouteSuite) TestRevertRoute() {
	routeManager := makeRevertRouteManager()
	user := &user.DBUser{Id: "userName"}
	ctx := gimlet.AttachUser(context.Background(), user)
	s.NotNil(routeManager)
	changes := restModel.APIAdminSettings{
		Banner: utility.ToStringPtr("foo"),
	}
	before := testutil.NewEnvironment(ctx, s.T()).Settings()
	_, err := data.SetEvergreenSettings(ctx, &changes, before, user, true)
	s.NoError(err)
	dbEvents, err := event.FindAdmin(s.T().Context(), event.RecentAdminEvents(1))
	s.NoError(err)
	s.GreaterOrEqual(len(dbEvents), 1)
	eventData := dbEvents[0].Data.(*event.AdminEventData)
	guid := eventData.GUID
	s.NotEmpty(guid)

	body := struct {
		GUID string `json:"guid"`
	}{guid}
	jsonBody, err := json.Marshal(&body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/admin/revert", buffer)
	s.NoError(err)
	err = routeManager.Parse(ctx, request)
	s.NoError(err)
	resp := routeManager.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	body = struct {
		GUID string `json:"guid"`
	}{""}
	jsonBody, err = json.Marshal(&body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/admin/revert", buffer)
	s.NoError(err)
	err = routeManager.Parse(ctx, request)
	s.Error(err)
	s.NotNil(ctx)
}

func (s *AdminRouteSuite) TestRestartTasksRoute() {
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "userName"})

	queue := s.env.LocalQueue()
	handler := makeRestartRoute(queue)

	s.NotNil(handler)

	startTime := time.Date(2017, time.June, 12, 11, 0, 0, 0, time.Local)
	endTime := time.Date(2017, time.June, 12, 13, 0, 0, 0, time.Local)

	// test that invalid time range errors
	body := struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		DryRun    bool      `json:"dry_run"`
	}{endTime, startTime, true}
	jsonBody, err := json.Marshal(&body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/admin/restart/tasks", buffer)
	s.NoError(err)
	s.Error(handler.Parse(ctx, request))

	// test a valid request
	body = struct {
		StartTime time.Time `json:"start_time"`
		EndTime   time.Time `json:"end_time"`
		DryRun    bool      `json:"dry_run"`
	}{startTime, endTime, true}
	jsonBody, err = json.Marshal(&body)
	s.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest(http.MethodPost, "/admin/restart/tasks", buffer)
	s.NoError(err)
	s.NoError(handler.Parse(ctx, request))
	resp := handler.Run(ctx)
	s.NotNil(resp)
	model, ok := resp.Data().(*restModel.RestartResponse)
	s.True(ok)
	s.NotEmpty(model.ItemsRestarted)
	s.Nil(model.ItemsErrored)
}

func (s *AdminRouteSuite) TestAdminEventRoute() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(db.ClearCollections(evergreen.ConfigCollection, event.EventCollection, distro.Collection), "Error clearing collections")

	// sd by test to have a valid distro in the collection
	d1 := &distro.Distro{
		Id: "valid-distro",
	}
	s.NoError(d1.Insert(ctx))

	// log some changes in the event log with the /admin/settings route
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})
	routeManager := makeSetAdminSettings()

	testSettings := testutil.MockConfig()
	jsonBody, err := json.Marshal(testSettings)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest(http.MethodPost, "/admin/settings", buffer)
	s.NoError(err)
	s.NoError(routeManager.Parse(ctx, request))
	now := time.Now()
	resp := routeManager.Run(ctx)
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())

	// get the changes with the /admin/events route
	ctx = context.Background()
	route := makeFetchAdminEvents()
	request, err = http.NewRequest(http.MethodGet, "/admin/events?limit=10&ts=3026-01-02T15%3A04%3A05Z", nil)
	s.NoError(err)
	s.NoError(route.Parse(ctx, request))
	response := route.Run(ctx)
	s.NotNil(resp)
	count := 0

	data := response.Data().([]any)
	for _, model := range data {
		evt, ok := model.(*restModel.APIAdminEvent)
		s.True(ok)
		count++
		s.NotEmpty(evt.Guid)
		s.NotNil(evt.Before)
		s.NotNil(evt.After)
		s.Equal("user", evt.User)
	}
	s.Equal(10, count)
	pagination := response.Pages()
	s.NotNil(pagination)
	s.NotNil(pagination.Next)
	s.NotZero(pagination.Next.KeyQueryParam)
	s.Equal("limit", pagination.Next.LimitQueryParam)
	s.Equal("next", pagination.Next.Relation)
	s.Equal(10, pagination.Next.Limit)
	ts, err := time.Parse(time.RFC3339, pagination.Next.Key)
	s.NoError(err)
	s.InDelta(now.Unix(), ts.Unix(), float64(time.Millisecond.Nanoseconds()))
}

func (s *AdminRouteSuite) TestClearTaskQueueRoute() {
	route := &clearTaskQueueHandler{}
	distro := "d1"
	tasks := []model.TaskQueueItem{
		{
			Id: "task1",
		},
		{
			Id: "task2",
		},
		{
			Id: "task3",
		},
	}
	queue := model.NewTaskQueue(distro, tasks, model.DistroQueueInfo{})
	s.Len(queue.Queue, 3)
	s.NoError(queue.Save(s.T().Context()))

	route.distro = distro
	resp := route.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())

	queueFromDb, err := model.LoadTaskQueue(s.T().Context(), distro)
	s.NoError(err)
	s.Empty(queueFromDb.Queue)
}
