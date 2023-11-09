package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type cliVersion struct{}

func makeFetchCLIVersionRoute() gimlet.RouteHandler {
	return &cliVersion{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch CLI client version
//	@Description	Fetch the CLI update manifest from the server
//	@Tags			info
//	@Router			/status/cli_version [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APICLIUpdate
func (gh *cliVersion) Factory() gimlet.RouteHandler {
	return &cliVersion{}
}

func (gh *cliVersion) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (gh *cliVersion) Run(ctx context.Context) gimlet.Responder {
	version, err := data.GetCLIUpdate(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting CLI updates"))
	}

	return gimlet.NewJSONResponse(version)
}
