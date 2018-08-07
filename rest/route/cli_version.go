package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

type cliVersion struct {
	sc data.Connector
}

func makeFetchCLIVersionRoute(sc data.Connector) gimlet.RouteHandler {
	return &cliVersion{
		sc: sc,
	}
}

func (gh *cliVersion) Factory() gimlet.RouteHandler {
	return &cliVersion{
		sc: gh.sc,
	}
}

func (gh *cliVersion) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (gh *cliVersion) Run(ctx context.Context) gimlet.Responder {
	version, err := gh.sc.GetCLIUpdate()
	if err != nil || version == nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(version)
}
