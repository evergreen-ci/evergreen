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

type AdminBannerRouteSuite struct {
	sc data.Connector
	suite.Suite
	postHandler MethodHandler
}

func TestAdminBannerRouteSuite(t *testing.T) {
	assert := assert.New(t)
	s := new(AdminBannerRouteSuite)
	s.sc = &data.MockConnector{}

	// test getting the route handler
	const route = "/admin/banner"
	const version = 2
	routeManager := getBannerRouteManager(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	s.postHandler = routeManager.Methods[0]
	assert.IsType(&bannerPostHandler{}, s.postHandler.RequestHandler)

	// run the rest of the tests
	suite.Run(t, s)
}

func (s *AdminBannerRouteSuite) TestAdminRoute() {
	ctx := context.Background()

	// test parsing the POST body
	body := model.APIBanner{
		Text: "hello evergreen users!",
	}
	jsonBody, err := json.Marshal(&body)
	s.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/banner", buffer)
	s.NoError(err)
	s.NoError(s.postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h := s.postHandler.RequestHandler.(*bannerPostHandler)
	s.Equal(body.Text, h.Banner)

	// test executing the POST request
	resp, err := s.postHandler.RequestHandler.Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(resp)
	settings, err := s.sc.GetAdminSettings()
	s.NoError(err)
	s.Equal(string(body.Text), settings.Banner)
}
