package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type AdminRouteSuite struct {
	sc data.Connector
	suite.Suite
	getHandler  MethodHandler
	postHandler MethodHandler
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

	// test parsing the POST body
	body := admin.AdminSettings{
		Banner:      "banner text",
		BannerTheme: admin.Information,
		ServiceFlags: admin.ServiceFlags{
			TaskDispatchDisabled: true,
			RepotrackerDisabled:  true,
		},
	}
	jsonBody, err := json.Marshal(&body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h := s.postHandler.RequestHandler.(*adminPostHandler)
	s.Equal(body.Banner, string(h.Banner))
	s.Equal(string(body.BannerTheme), string(h.BannerTheme))
	s.Equal(body.ServiceFlags.TaskDispatchDisabled, h.ServiceFlags.TaskDispatchDisabled)
	s.Equal(body.ServiceFlags.RepotrackerDisabled, h.ServiceFlags.RepotrackerDisabled)

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
	settings, ok := settingsResp.(admin.AdminSettings)
	s.True(ok)
	s.Equal(body.Banner, settings.Banner)
	s.Equal(body.BannerTheme, settings.BannerTheme)
	s.Equal(body.ServiceFlags.TaskDispatchDisabled, settings.ServiceFlags.TaskDispatchDisabled)
	s.Equal(body.ServiceFlags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
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
	routeManager := getRestartRouteManager(route, version)
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
	assert.NotNil(model.TasksErrored)
}
