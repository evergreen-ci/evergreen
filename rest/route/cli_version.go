package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

type cliVersion struct{}

func makeFetchCLIVersionRoute() gimlet.RouteHandler {
	return &cliVersion{}
}

func (gh *cliVersion) Factory() gimlet.RouteHandler {
	return &cliVersion{}
}

func (gh *cliVersion) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (gh *cliVersion) Run(ctx context.Context) gimlet.Responder {
	dc := data.CLIUpdateConnector{}
	version, err := dc.GetCLIUpdate()
	if err != nil || version == nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(version)
}
