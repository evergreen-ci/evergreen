package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
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
	dc := data.DBHostConnector{}
	res, err := dc.AggregateSpawnhostData()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(res)
}
