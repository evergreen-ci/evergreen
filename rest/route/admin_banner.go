package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// this manages the /admin/banner route, which allows setting the banner
func getBannerRouteManager(route string, version int) *RouteManager {
	bph := &bannerPostHandler{}
	bannerPost := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: bph.Handler(),
		MethodType:     http.MethodPost,
	}

	bgh := &bannerGetHandler{}
	bannerGet := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: bgh.Handler(),
		MethodType:     http.MethodGet,
	}

	bannerRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{bannerPost, bannerGet},
		Version: version,
	}
	return &bannerRoute
}

type bannerPostHandler struct {
	Banner model.APIString `json:"banner"`
	Theme  model.APIString `json:"theme"`
	model  model.APIBanner
}

func (h *bannerPostHandler) Handler() RequestHandler {
	return &bannerPostHandler{}
}

func (h *bannerPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, h); err != nil {
		return err
	}
	h.model = model.APIBanner{
		Text:  h.Banner,
		Theme: h.Theme,
	}
	return nil
}

func (h *bannerPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	if err := sc.SetAdminBanner(model.FromAPIString(h.Banner), u); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	if err := sc.SetBannerTheme(model.FromAPIString(h.Theme), u); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&h.model},
	}, nil
}

type bannerGetHandler struct{}

func (h *bannerGetHandler) Handler() RequestHandler {
	return &bannerGetHandler{}
}

func (h *bannerGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *bannerGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	banner, theme, err := sc.GetBanner()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{&model.APIBanner{Text: model.ToAPIString(banner), Theme: model.ToAPIString(theme)}},
	}, nil
}
