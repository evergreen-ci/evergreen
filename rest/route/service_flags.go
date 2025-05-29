package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
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
	flags, err := data.GetNecessaryServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error retrieving service flags"))
	}

	return gimlet.NewJSONResponse(flags)
}
