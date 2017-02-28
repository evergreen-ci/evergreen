package route

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/gorilla/mux"
)

// Route defines all of the functioning of a particular API route. It contains
// implementations of the various API methods that are defined on this endpoint.
type RouteManager struct {
	// Methods is a slice containing all of the http methods (PUT, GET, DELETE, etc.)
	// for this route.
	Methods []MethodHandler

	// Route is the path in the url that this resource handles.
	Route string

	// Version is the version number of the API that this route is associated with.
	Version int
}

// Register builds http handlers for each of the defined methods and attaches
// these to the given router.
func (rm *RouteManager) Register(r *mux.Router, sc servicecontext.ServiceContext) {
	for _, method := range rm.Methods {
		routeHandlerFunc := makeHandler(method, sc, rm.Route, rm.Version)
		sr := r.PathPrefix(fmt.Sprintf("/%s/v%d/", sc.GetPrefix(), rm.Version)).Subrouter().StrictSlash(true)

		sr.HandleFunc(rm.Route, routeHandlerFunc).Methods(method.MethodType)
	}
}
