package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeFetchSpawnHostUsage() gimlet.RouteHandler {
	return &adminSpawnHostHandler{}
}

type adminSpawnHostHandler struct{}

func (h *adminSpawnHostHandler) Factory() gimlet.RouteHandler {
	return &adminSpawnHostHandler{}
}

func (h *adminSpawnHostHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminSpawnHostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := host.AggregateSpawnhostData(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting spawn host usage data"))
	}
	return gimlet.NewJSONResponse(res)
}
