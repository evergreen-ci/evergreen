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
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetBanner(t *testing.T) {
	assert := assert.New(t)
	sc := &data.DBConnector{}

	// test getting the route handler
	routeManager := makeSetAdminBanner(sc)
	assert.NotNil(routeManager)
	assert.IsType(&bannerPostHandler{}, routeManager)

	// run the route
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "user"})

	// test changing the banner with no change in theme
	body := model.APIBanner{
		Text: utility.ToStringPtr("hello evergreen users!"),
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
	assert.Equal(utility.FromStringPtr(body.Text), settings.Banner)

	// test changing the theme
	body = model.APIBanner{
		Text:  utility.ToStringPtr("banner is changing again"),
		Theme: utility.ToStringPtr("important"),
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
	assert.Equal(utility.FromStringPtr(body.Theme), string(settings.BannerTheme))

	// test invalid theme enum
	body = model.APIBanner{
		Text:  utility.ToStringPtr(""),
		Theme: utility.ToStringPtr("foo"),
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
	connector := data.DBAdminConnector{}
	u := &user.DBUser{
		Id: evergreen.ParentPatchUser,
	}
	newSettings := &model.APIAdminSettings{
		Banner:      utility.ToStringPtr("foo"),
		BannerTheme: utility.ToStringPtr("warning"),
		ApiUrl:      utility.ToStringPtr("test"),
		ConfigDir:   utility.ToStringPtr("test"),
		AuthConfig: &model.APIAuthConfig{
			Github: &model.APIGithubAuthConfig{
				Organization: utility.ToStringPtr("test"),
			},
		},
		Ui: &model.APIUIConfig{
			Secret:         utility.ToStringPtr("test"),
			Url:            utility.ToStringPtr("test"),
			DefaultProject: utility.ToStringPtr("test"),
		},
		Providers: &model.APICloudProviders{
			AWS: &model.APIAWSConfig{
				Pod: &model.APIAWSPodConfig{
					ECS: &model.APIECSConfig{},
				},
			},
			Docker: &model.APIDockerConfig{
				APIVersion:    utility.ToStringPtr(""),
				DefaultDistro: utility.ToStringPtr(""),
			},
			GCE: &model.APIGCEConfig{
				ClientEmail:  utility.ToStringPtr("gce_email"),
				PrivateKey:   utility.ToStringPtr("gce_key"),
				PrivateKeyID: utility.ToStringPtr("gce_key_id"),
				TokenURI:     utility.ToStringPtr("gce_token"),
			},
			OpenStack: &model.APIOpenStackConfig{
				IdentityEndpoint: utility.ToStringPtr("endpoint"),
				Username:         utility.ToStringPtr("username"),
				Password:         utility.ToStringPtr("password"),
				DomainName:       utility.ToStringPtr("domain"),
				ProjectName:      utility.ToStringPtr("project"),
				ProjectID:        utility.ToStringPtr("project_id"),
				Region:           utility.ToStringPtr("region"),
			},
			VSphere: &model.APIVSphereConfig{
				Host:     utility.ToStringPtr("host"),
				Username: utility.ToStringPtr("vsphere"),
				Password: utility.ToStringPtr("vsphere_pass"),
			},
		},
	}
	_, err := connector.SetEvergreenSettings(newSettings, &evergreen.Settings{}, u, true)
	require.NoError(t, err)
	routeManager := makeFetchAdminBanner(&data.DBConnector{
		DBAdminConnector: connector,
	})
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
	assert.Equal("foo", utility.FromStringPtr(banner.Text))
	assert.Equal("warning", utility.FromStringPtr(banner.Theme))
}
