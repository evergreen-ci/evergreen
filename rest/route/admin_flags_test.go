package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type AdminFlagsRouteSuite struct {
	sc data.Connector
	suite.Suite
	postHandler MethodHandler
}

func TestAdminFlagsRouteSuite(t *testing.T) {
	assert := assert.New(t)
	s := new(AdminFlagsRouteSuite)
	s.sc = &data.MockConnector{}

	// test getting the route handler
	const route = "/admin/service_flags"
	const version = 2
	routeManager := getServiceFlagsRouteManager(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	s.postHandler = routeManager.Methods[0]
	assert.IsType(&flagsPostHandler{}, s.postHandler.RequestHandler)

	// run the rest of the tests
	suite.Run(t, s)
}

func (s *AdminFlagsRouteSuite) TestAdminRoute() {
	ctx := context.Background()

	// test parsing the POST body
	body := struct {
		Flags model.APIServiceFlags `json:"service_flags"`
	}{
		Flags: model.APIServiceFlags{
			HostinitDisabled:   true,
			TaskrunnerDisabled: true,
		},
	}
	jsonBody, err := json.Marshal(&body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/service_flags", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h := s.postHandler.RequestHandler.(*flagsPostHandler)
	s.Equal(body.Flags, h.Flags)

	// test executing the POST request
	resp, err := s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)
	settings, err := s.sc.GetAdminSettings()
	s.NoError(err)
	s.Equal(body.Flags.HostinitDisabled, settings.ServiceFlags.HostinitDisabled)
	s.Equal(body.Flags.TaskrunnerDisabled, settings.ServiceFlags.TaskrunnerDisabled)
	s.Equal(body.Flags.RepotrackerDisabled, settings.ServiceFlags.RepotrackerDisabled)
}
