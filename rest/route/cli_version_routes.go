package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
)

type cliVersion struct {
}

func getCLIVersionRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			MethodHandler{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &cliVersion{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (gh *cliVersion) Handler() RequestHandler {
	return &cliVersion{}
}

func (gh *cliVersion) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}

func (gh *cliVersion) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	version, err := sc.GetCLIUpdate()
	resp := ResponseData{}
	if err == nil && version != nil {
		resp.Result = append(resp.Result, version)
	}
	return resp, err
}
