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
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
)

func TestSetBanner(t *testing.T) {
	assert := assert.New(t)
	sc := &data.MockConnector{}

	// test getting the route handler
	routeManager := makeSetAdminBanner(sc)
	assert.NotNil(routeManager)
	assert.IsType(&bannerPostHandler{}, routeManager)

	// run the route
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	// test changing the banner with no change in theme
	body := model.APIBanner{
		Text: model.ToStringPtr("hello evergreen users!"),
	}
	jsonBody, err := json.Marshal(&body)
	assert.NoError(err)
	buffer := bytes.NewBuffer(jsonBody)
	request, err := http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.NoError(err)
	h := routeManager.(*bannerPostHandler)
	assert.Equal(body.Text, h.Banner)
	resp := routeManager.Run(ctx)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	settings, err := sc.GetEvergreenSettings()
	assert.NoError(err)
	assert.Equal(model.FromStringPtr(body.Text), settings.Banner)

	// test changing the theme
	body = model.APIBanner{
		Text:  model.ToStringPtr("banner is changing again"),
		Theme: model.ToStringPtr("important"),
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.NoError(err)
	h = routeManager.(*bannerPostHandler)
	assert.Equal(body.Theme, h.Theme)
	resp = routeManager.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	settings, err = sc.GetEvergreenSettings()
	assert.NoError(err)
	assert.Equal(model.FromStringPtr(body.Theme), string(settings.BannerTheme))

	// test invalid theme enum
	body = model.APIBanner{
		Text:  model.ToStringPtr(""),
		Theme: model.ToStringPtr("foo"),
	}
	jsonBody, err = json.Marshal(&body)
	assert.NoError(err)
	buffer = bytes.NewBuffer(jsonBody)
	request, err = http.NewRequest("POST", "/admin/banner", buffer)
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.NoError(err)
	h = routeManager.(*bannerPostHandler)
	assert.Equal(body.Theme, h.Theme)
	resp = routeManager.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusBadRequest, resp.Status())
}

func TestFetchBanner(t *testing.T) {
	assert := assert.New(t)

	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "userName"})
	connector := &data.MockConnector{
		MockAdminConnector: data.MockAdminConnector{
			MockSettings: &evergreen.Settings{
				Banner:      "foo",
				BannerTheme: "warning",
			},
		},
	}
	routeManager := makeFetchAdminBanner(connector)
	assert.NotNil(routeManager)

	// test getting what we just sets
	request, err := http.NewRequest("GET", "/admin/banner", nil)
	assert.NoError(err)
	err = routeManager.Parse(ctx, request)
	assert.NoError(err)

	resp := routeManager.Run(ctx)
	assert.NoError(err)
	assert.NotNil(resp)

	modelInterface, err := resp.Data().(model.Model).ToService()
	assert.NoError(err)
	banner := modelInterface.(*model.APIBanner)
	assert.Equal("foo", model.FromStringPtr(banner.Text))
	assert.Equal("warning", model.FromStringPtr(banner.Theme))
}
