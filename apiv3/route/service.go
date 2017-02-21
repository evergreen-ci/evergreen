package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/gorilla/mux"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a ServiceContext then attaches each of the main functions for
// the api to the router.
func AttachHandler(root *mux.Router) http.Handler {
	sc := servicecontext.NewServiceContext()

	return getHandler(rtr, sc)
}

// getHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func getHandler(r *mux.Router, sc servicecontext.ServiceContext) http.Handler {
	placeHolderGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: &PlaceHolderRequestHandler{},
		MethodType:     evergreen.MethodGet,
	}

	placeHolderRoute := RouteManager{
		Route:   "/",
		Methods: []MethodHandler{placeHolderGet},
		Version: 2,
	}

	placeHolderRoute.Register(r, sc)

	return r
}

type PlaceHolderRequestHandler struct {
}

func (p *PlaceHolderRequestHandler) Handler() RequestHandler {
	return &PlaceHolderRequestHandler{}
}

func (p *PlaceHolderRequestHandler) Parse(r *http.Request) error {
	return nil
}

func (p *PlaceHolderRequestHandler) Validate() error {
	return nil
}

func (p *PlaceHolderRequestHandler) Execute(sc *servicecontext.ServiceContext) (model.Model, error) {
	return nil, apiv3.APIError{
		StatusCode: 200,
		Message:    "this is a placeholder for now",
	}
}
