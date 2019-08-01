package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeFetchAdminSettings(sc data.Connector) gimlet.RouteHandler {
	return &adminGetHandler{
		sc: sc,
	}
}

type adminGetHandler struct {
	sc data.Connector
}

func (h *adminGetHandler) Factory() gimlet.RouteHandler {
	return &adminGetHandler{
		sc: h.sc,
	}
}

func (h *adminGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := h.sc.GetEvergreenSettings()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}
	settingsModel := model.NewConfigModel()

	err = settingsModel.BuildFromService(settings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(settingsModel)
}

func makeSetAdminSettings(sc data.Connector) gimlet.RouteHandler {
	return &adminPostHandler{
		sc: sc,
	}
}

type adminPostHandler struct {
	model *model.APIAdminSettings
	sc    data.Connector
}

func (h *adminPostHandler) Factory() gimlet.RouteHandler {
	return &adminPostHandler{sc: h.sc}
}

func (h *adminPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.Wrap(gimlet.GetJSON(r.Body, &h.model), "error parsing request body")
}

func (h *adminPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	oldSettings, err := h.sc.GetEvergreenSettings()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error retrieving existing settings"))
	}

	// validate the changes
	newSettings, err := h.sc.SetEvergreenSettings(h.model, oldSettings, u, false)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error applying new settings"))
	}

	err = newSettings.Validate()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Validation error"))
	}

	err = distro.ValidateContainerPoolDistros(newSettings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Validation error"))
	}

	_, err = h.sc.SetEvergreenSettings(h.model, oldSettings, u, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	err = h.model.BuildFromService(newSettings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error building API model"))
	}

	return gimlet.NewJSONResponse(h.model)
}

func makeFetchBugsnag(sc data.Connector) gimlet.RouteHandler {
	return &getBugsnagTokenHandler{
		sc: sc,
	}
}

type getBugsnagTokenHandler struct {
	sc data.Connector
}

func (h *getBugsnagTokenHandler) Factory() gimlet.RouteHandler {
	return &getBugsnagTokenHandler{
		sc: h.sc,
	}
}

func (h *getBugsnagTokenHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *getBugsnagTokenHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := h.sc.GetEvergreenSettings()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewTextResponse(settings.Bugsnag)
}
