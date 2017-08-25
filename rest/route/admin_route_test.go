package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
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

	// test parsing the POST body
	body := admin.AdminSettings{
		Banner: "banner text",
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
	s.Equal(body.ServiceFlags.TaskDispatchDisabled, settings.ServiceFlags.TaskDispatchDisabled)
	s.Equal(body.ServiceFlags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
}

func (s *AdminRouteSuite) TestGetAuthentication() {
	s.DoAuthenticationTests(s.getHandler.Authenticate)
}

func (s *AdminRouteSuite) TestPostAuthentication() {
	s.DoAuthenticationTests(s.postHandler.Authenticate)
}

func (s *AdminRouteSuite) DoAuthenticationTests(authFunc func(context.Context, data.Connector) error) {
	superUser := user.DBUser{
		Id: "super_user",
	}
	normalUser := user.DBUser{
		Id: "normal_user",
	}
	s.sc.SetSuperUsers([]string{"super_user"})
	superCtx := context.WithValue(context.Background(), RequestUser, &superUser)
	normalCtx := context.WithValue(context.Background(), RequestUser, &normalUser)

	s.NoError(authFunc(superCtx, s.sc))
	s.Error(authFunc(normalCtx, s.sc))
}
