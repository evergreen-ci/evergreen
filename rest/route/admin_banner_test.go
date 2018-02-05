package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/assert"
)

func TestAdminBannerRoute(t *testing.T) {
	assert := assert.New(t)
	sc := &data.MockConnector{}

	// test getting the route handler
	const route = "/admin/banner"
	const version = 2
	routeManager := getBannerRouteManager(route, version)
	assert.NotNil(routeManager)
	assert.Equal(route, routeManager.Route)
	assert.Equal(version, routeManager.Version)
	postHandler := routeManager.Methods[0]
	assert.IsType(&bannerPostHandler{}, postHandler.RequestHandler)

	// run the route
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "user"})

	// test changing the banner with no change in theme
	body := model.APIBanner{
		Text: "hello evergreen users!",
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h := postHandler.RequestHandler.(*bannerPostHandler)
	assert.Equal(body.Text, h.Banner)
	resp, err := postHandler.RequestHandler.Execute(ctx, sc)
	assert.NoError(err)
	assert.NotNil(resp)
	settings, err := sc.GetEvergreenSettings()
	assert.NoError(err)
	assert.Equal(string(body.Text), settings.Banner)

	// test changing the theme
	body = model.APIBanner{
		Text:  "banner is changing again",
		Theme: "important",
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h = postHandler.RequestHandler.(*bannerPostHandler)
	assert.Equal(body.Theme, h.Theme)
	resp, err = postHandler.RequestHandler.Execute(ctx, sc)
	assert.NoError(err)
	assert.NotNil(resp)
	settings, err = sc.GetEvergreenSettings()
	assert.NoError(err)
	assert.Equal(string(body.Theme), string(settings.BannerTheme))

	// test invalid theme enum
	body = model.APIBanner{
		Text:  "",
		Theme: "foo",
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	assert.NoError(postHandler.RequestHandler.ParseAndValidate(ctx, request))
	h = postHandler.RequestHandler.(*bannerPostHandler)
	assert.Equal(body.Theme, h.Theme)
	_, err = postHandler.RequestHandler.Execute(ctx, sc)
	assert.Error(err)
}
