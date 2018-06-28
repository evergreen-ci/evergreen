package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// this route is to preserve backwards compatibility with old versions of the CLI
func getLegacyAdminSettingsManager(route string, version int) *RouteManager {
	agh := &legacyAdminGetHandler{}
	adminGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: agh.Handler(),
		MethodType:     http.MethodGet,
	}

	adminRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{adminGet},
		Version: version,
	}
	return &adminRoute
}

type legacyAdminGetHandler struct{}

func (h *legacyAdminGetHandler) Handler() RequestHandler {
	return &legacyAdminGetHandler{}
}

func (h *legacyAdminGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *legacyAdminGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	settings, err := sc.GetEvergreenSettings()
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	settingsModel := model.NewConfigModel()
	err = settingsModel.BuildFromService(settings)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}
	modelCopy := model.NewConfigModel()
	modelCopy.Banner = settingsModel.Banner
	modelCopy.BannerTheme = settingsModel.BannerTheme
	return ResponseData{
		Result: []model.Model{modelCopy},
	}, nil
}
