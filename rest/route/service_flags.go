package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
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
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting service flags"))
	}

	return gimlet.NewJSONResponse(model.APIServiceFlagsResponse{
		DebugSpawnHostDisabled:      flags.DebugSpawnHostDisabled,
		CrossFileYAMLAnchorsEnabled: flags.CrossFileYAMLAnchorsEnabled,
	})
}
