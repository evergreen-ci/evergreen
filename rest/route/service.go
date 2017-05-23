package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/servicecontext"
	"github.com/gorilla/mux"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a ServiceContext then attaches each of the main functions for
// the api to the router.
func AttachHandler(root *mux.Router, superUsers []string, URL, prefix string) http.Handler {
	sc := &servicecontext.DBServiceContext{}

	sc.SetURL(URL)
	sc.SetPrefix(prefix)
	sc.SetSuperUsers(superUsers)
	return GetHandler(root, sc)
}

// GetHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func GetHandler(r *mux.Router, sc servicecontext.ServiceContext) http.Handler {
	// PLACE HOLDER ROUTE DEFINITION
	// make object
	placeHolderGet := MethodHandler{
		Authenticator: &NoAuthAuthenticator{},
		// call handler
		RequestHandler: &PlaceHolderRequestHandler{},
		MethodType:     evergreen.MethodGet,
	}

	placeHolderRoute := RouteManager{
		Route:   "/",
		Methods: []MethodHandler{placeHolderGet},
		Version: 2,
	}

	getHostRouteManager("/hosts", 2).Register(r, sc)
	getTaskRouteManager("/tasks/{task_id}", 2).Register(r, sc)
	getTestRouteManager("/tasks/{task_id}/tests", 2).Register(r, sc)
	getTasksByProjectAndCommitRouteManager("/projects/{project_id}/revisions/{commit_hash}/tasks", 2).Register(r, sc)
	getTasksByBuildRouteManager("/builds/{build_id}/tasks", 2).Register(r, sc)
	getTaskRestartRouteManager("/tasks/{task_id}/restart", 2).Register(r, sc)
	placeHolderRoute.Register(r, sc)
	return r
}

type PlaceHolderRequestHandler struct {
}

func (p *PlaceHolderRequestHandler) Handler() RequestHandler {
	return &PlaceHolderRequestHandler{}
}

func (p *PlaceHolderRequestHandler) ParseAndValidate(r *http.Request) error {
	return nil
}
func (p *PlaceHolderRequestHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	return ResponseData{}, rest.APIError{
		StatusCode: 200,
		Message:    "this is a placeholder for now",
	}
}
