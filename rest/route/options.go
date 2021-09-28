package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

// optionsHandler is a RequestHandler for resolving options requests.
type optionsHandler struct {
}

func makeOptionsHandler() gimlet.RouteHandler {
	return &optionsHandler{}
}

// Handler returns a pointer to a new optionsHandler.
func (h *optionsHandler) Factory() gimlet.RouteHandler {
	return &optionsHandler{}
}

func (h *optionsHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

// Execute returns an empty json response to the options request.
// The options request only looks for a 200 status code so the response value is not relevant.
func (h *optionsHandler) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(struct{}{})
}
