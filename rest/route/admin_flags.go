package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeSetServiceFlagsRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &flagsPostHandler{
		sc: sc,
	}
}

type flagsPostHandler struct {
	Flags model.APIServiceFlags `json:"service_flags"`
	sc    data.Connector
}

func (h *flagsPostHandler) Factory() gimlet.RouteHandler {
	return &flagsPostHandler{
		sc: h.sc,
	}
}

func (h *flagsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.Wrap(gimlet.GetJSON(r.Body, h), "problem parsing request body")
}

func (h *flagsPostHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	flags, err := h.Flags.ToService()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	err = h.sc.SetServiceFlags(flags.(evergreen.ServiceFlags), u)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(h.Flags)
}
