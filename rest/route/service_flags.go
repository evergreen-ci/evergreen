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
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "getting service flags"))
	}
	apiFlags := &model.APIServiceFlags{}
	if err := apiFlags.BuildFromService(*flags); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "building service flags response"))
	}
	return gimlet.NewJSONResponse(apiFlags)
}
