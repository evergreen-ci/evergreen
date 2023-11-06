package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func makeSetAdminBanner() gimlet.RouteHandler {
	return &bannerPostHandler{}
}

type bannerPostHandler struct {
	Banner *string `json:"banner"`
	Theme  *string `json:"theme"`
	model  model.APIBanner
}

func (h *bannerPostHandler) Factory() gimlet.RouteHandler {
	return &bannerPostHandler{}
}

func (h *bannerPostHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.Wrap(err, "problem parsing request")
	}

	h.model = model.APIBanner{
		Text:  h.Banner,
		Theme: h.Theme,
	}

	return nil
}

func (h *bannerPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	if err := evergreen.SetBanner(ctx, utility.FromStringPtr(h.Banner)); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "setting banner text"))
	}
	if err := data.SetBannerTheme(ctx, utility.FromStringPtr(h.Theme), u); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "setting banner theme"))
	}

	return gimlet.NewJSONResponse(h.model)
}

func makeFetchAdminBanner() gimlet.RouteHandler {
	return &bannerGetHandler{}
}

type bannerGetHandler struct {
}

// Factory creates an instance of the handler.
//
//	@Summary		Get the banner
//	@Description	Fetch the text and type of Evergreen's current banner
//	@Tags			info
//	@Router			/admin/banner [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APIBanner
func (h *bannerGetHandler) Factory() gimlet.RouteHandler {
	return &bannerGetHandler{}
}

func (h *bannerGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *bannerGetHandler) Run(ctx context.Context) gimlet.Responder {
	banner, theme, err := data.GetBanner(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting banner"))
	}

	return gimlet.NewJSONResponse(&model.APIBanner{
		Text:  utility.ToStringPtr(banner),
		Theme: utility.ToStringPtr(theme),
	})
}
