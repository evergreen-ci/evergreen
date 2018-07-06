package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// this route is to preserve backwards compatibility with old versions of the CLI
func makeLegacyAdminConfig(sc data.Connector) gimlet.RouteHandler {
	return &legacyAdminGetHandler{
		sc: sc,
	}
}

type legacyAdminGetHandler struct {
	sc data.Connector
}

func (h *legacyAdminGetHandler) Factory() gimlet.RouteHandler {
	return &legacyAdminGetHandler{
		sc: h.sc,
	}
}

func (h *legacyAdminGetHandler) Parse(ctx context.Context, r *http.Request) (context.Context, error) {
	return ctx, nil
}

func (h *legacyAdminGetHandler) Run(ctx context.Context) gimlet.Responder {
	settings, err := h.sc.GetEvergreenSettings()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	settingsModel := model.NewConfigModel()

	if err = settingsModel.BuildFromService(settings); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	modelCopy := model.NewConfigModel()
	modelCopy.Banner = settingsModel.Banner
	modelCopy.BannerTheme = settingsModel.BannerTheme

	return gimlet.NewJSONResponse(modelCopy)
}
