package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/ip_address/{ip_address}

type addressGetHandler struct {
	IP   string
	Host *host.Host
	sc   data.Connector
}

func makeGetHostByIP(sc data.Connector) gimlet.RouteHandler {
	return &addressGetHandler{
		sc: sc,
	}
}

func (h *addressGetHandler) Factory() gimlet.RouteHandler {
	return &addressGetHandler{
		sc: h.sc,
	}
}
func (h *addressGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.IP = gimlet.GetVars(r)["ip_address"]

	if h.IP == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (h *addressGetHandler) Run(ctx context.Context) gimlet.Responder {
	host, err := h.sc.FindHostByIP(h.IP)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error fetching host information"))
	}
	if host == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with ip '%s' not found", h.IP),
		})
	}

	return gimlet.NewJSONResponse(host)
}
