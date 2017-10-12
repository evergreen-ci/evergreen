package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
)

func getPlaceHolderManger(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &placeHolderHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

type placeHolderHandler struct{}

func (p *placeHolderHandler) Handler() RequestHandler {
	return &placeHolderHandler{}
}

func (p *placeHolderHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return nil
}
func (p *placeHolderHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	return ResponseData{}, rest.APIError{
		StatusCode: 200,
		Message:    "this is a placeholder for now",
	}
}
