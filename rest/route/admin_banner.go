package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeSetAdminBanner(sc data.Connector) gimlet.RouteHandler {
	return &bannerPostHandler{
		sc: sc,
	}
}

type bannerPostHandler struct {
	Banner *string `json:"banner"`
	Theme  *string `json:"theme"`
	model  model.APIBanner

	sc data.Connector
}

func (h *bannerPostHandler) Factory() gimlet.RouteHandler {
	return &bannerPostHandler{
		sc: h.sc,
	}
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

	if err := h.sc.SetAdminBanner(model.FromStringPtr(h.Banner), u); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem setting banner text"))
	}
	if err := h.sc.SetBannerTheme(model.FromStringPtr(h.Theme), u); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem setting banner theme"))
	}

	return gimlet.NewJSONResponse(h.model)
}

func makeFetchAdminBanner(sc data.Connector) gimlet.RouteHandler {
	return &bannerGetHandler{
		sc: sc,
	}
}

type bannerGetHandler struct {
	sc data.Connector
}

func (h *bannerGetHandler) Factory() gimlet.RouteHandler {
	return &bannerGetHandler{
		sc: h.sc,
	}
}

func (h *bannerGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *bannerGetHandler) Run(ctx context.Context) gimlet.Responder {
	banner, theme, err := h.sc.GetBanner()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(&model.APIBanner{
		Text:  model.ToStringPtr(banner),
		Theme: model.ToStringPtr(theme),
	})
}
