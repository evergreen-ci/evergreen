package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// this manages the /admin/settings route, which allows getting/setting the admin settings
func getAdminSettingsManager(route string, version int) *RouteManager {
	agh := &adminGetHandler{}
	adminGet := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: agh.Handler(),
		MethodType:     http.MethodGet,
	}

	aph := &adminPostHandler{}
	adminPost := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: aph.Handler(),
		MethodType:     http.MethodPost,
	}

	adminRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{adminGet, adminPost},
		Version: version,
	}
	return &adminRoute
}

type adminGetHandler struct{}

func (h *adminGetHandler) Handler() RequestHandler {
	return &adminGetHandler{}
}

func (h *adminGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
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
	return ResponseData{
		Result: []model.Model{settingsModel},
	}, nil
}

type adminPostHandler struct {
	model *model.APIAdminSettings
}

func (h *adminPostHandler) Handler() RequestHandler {
	return &adminPostHandler{}
}

func (h *adminPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return errors.Wrap(util.ReadJSONInto(r.Body, &h.model), "error parsing request body")
}

func (h *adminPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)
	oldSettings, err := sc.GetEvergreenSettings()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error retrieving existing settings")
	}

	// validate the changes
	newSettings, err := sc.SetEvergreenSettings(h.model, oldSettings, u, false)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error applying new settings")
	}
	err = newSettings.Validate()
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Validation error")
	}
	err = distro.ValidateContainerPoolDistros(newSettings)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Validation error")
	}

	_, err = sc.SetEvergreenSettings(h.model, oldSettings, u, true)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	err = h.model.BuildFromService(newSettings)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error building API model")
	}
	return ResponseData{
		Result: []model.Model{h.model},
	}, nil
}
