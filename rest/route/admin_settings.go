package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func makeFetchAdminSettings() gimlet.RouteHandler {
	return &adminGetHandler{}
}

type adminGetHandler struct{}

func (h *adminGetHandler) Factory() gimlet.RouteHandler {
	return &adminGetHandler{}
}

func (h *adminGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig()
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

func makeFetchAdminUIV2Url() gimlet.RouteHandler {
	return &uiV2URLGetHandler{}
}

type uiV2URLGetHandler struct{}

func (h *uiV2URLGetHandler) Factory() gimlet.RouteHandler {
	return &uiV2URLGetHandler{}
}

func (h *uiV2URLGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *uiV2URLGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(&model.APIUiV2URL{
		UIv2Url: utility.ToStringPtr(settings.Ui.UIv2Url),
	})
}

func makeSetAdminSettings() gimlet.RouteHandler {
	return &adminPostHandler{}
}

type adminPostHandler struct {
	model *model.APIAdminSettings
}

func (h *adminPostHandler) Factory() gimlet.RouteHandler {
	return &adminPostHandler{}
}

func (h *adminPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.Wrap(gimlet.GetJSON(r.Body, &h.model), "error parsing request body")
}

func (h *adminPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	oldSettings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error retrieving existing settings"))
	}

	// validate the changes
	newSettings, err := data.SetEvergreenSettings(h.model, oldSettings, u, false)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error applying new settings"))
	}
	if err = newSettings.Validate(); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Validation error"))
	}

	err = distro.ValidateContainerPoolDistros(newSettings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Validation error"))
	}

	_, err = data.SetEvergreenSettings(h.model, oldSettings, u, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	err = h.model.BuildFromService(newSettings)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error building API model"))
	}

	return gimlet.NewJSONResponse(h.model)
}
