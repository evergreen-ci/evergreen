package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

func makeFetchSpawnHostUsage(sc data.Connector) gimlet.RouteHandler {
	return &adminSpawnHostHandler{sc: sc}
}

type adminSpawnHostHandler struct {
	sc data.Connector
}

func (h *adminSpawnHostHandler) Factory() gimlet.RouteHandler {
	return &adminSpawnHostHandler{
		sc: h.sc,
	}
}

func (h *adminSpawnHostHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *adminSpawnHostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := host.AggregateSpawnhostData()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(res)
}
