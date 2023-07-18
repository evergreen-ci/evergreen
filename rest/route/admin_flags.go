package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeSetServiceFlagsRouteManager() gimlet.RouteHandler {
	return &flagsPostHandler{}
}

type flagsPostHandler struct {
	Flags model.APIServiceFlags `json:"service_flags"`
}

func (h *flagsPostHandler) Factory() gimlet.RouteHandler {
	return &flagsPostHandler{}
}

func (h *flagsPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.Wrap(gimlet.GetJSON(r.Body, h), "parsing request body")
}

func (h *flagsPostHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := h.Flags.ToService()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "converting service flags to service model"))
	}

	err = evergreen.SetServiceFlagsContext(ctx, flags.(evergreen.ServiceFlags))
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "setting service flags"))
	}

	return gimlet.NewJSONResponse(h.Flags)
}
