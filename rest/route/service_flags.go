package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

type serviceFlagsGetHandler struct{}

func makeFetchServiceFlags() gimlet.RouteHandler {
	return &serviceFlagsGetHandler{}
}

func (h *serviceFlagsGetHandler) Factory() gimlet.RouteHandler {
	return &serviceFlagsGetHandler{}
}

func (h *serviceFlagsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *serviceFlagsGetHandler) Run(ctx context.Context) gimlet.Responder {
	// TODO (DEVPROD-17405): Remove this route once unsupported CLIs no longer
	// need the legacy service flags payload.
	flags := struct {
		StaticAPIKeysDisabled bool `json:"static_api_keys_disabled"`
	}{
		StaticAPIKeysDisabled: true,
	}

	return gimlet.NewJSONResponse(flags)
}
