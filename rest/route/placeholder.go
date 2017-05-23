package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/servicecontext"
)

func getPlaceHolderManger(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &placeHolderHandler{},
				MethodType:     evergreen.MethodGet,
			},
		},
		Version: version,
	}
}

type placeHolderHandler struct{}

func (p *placeHolderHandler) Handler() RequestHandler {
	return &placeHolderHandler{}
}

func (p *placeHolderHandler) ParseAndValidate(r *http.Request) error {
	return nil
}
func (p *placeHolderHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	return ResponseData{}, rest.APIError{
		StatusCode: 200,
		Message:    "this is a placeholder for now",
	}
}
