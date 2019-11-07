package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

/*
type SpawnHostUsageByUser struct {
	User            string
	SpawnHosts      HostData
	NumEBSVolumes   int
	TotalVolumeSize int
}

type HostData struct {
	Uptime     time.Duration
	Expiration time.Time
	Type       string
}
*/

func makeFetchSpawnHostUsage(sc data.Connector) gimlet.RouteHandler {
	return &adminSpawnHostHandler{sc: sc}
}

type adminSpawnHostHandler struct {
	//userID string

	sc data.Connector
}

func (h *adminSpawnHostHandler) Factory() gimlet.RouteHandler {
	return &adminSpawnHostHandler{
		sc: h.sc,
	}
}

func (h *adminSpawnHostHandler) Parse(ctx context.Context, r *http.Request) error {
	//h.userID = gimlet.GetVars(r)["user_id"]
	return nil
}

func (h *adminSpawnHostHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := host.AggregateSpawnhostData()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(res)
}
