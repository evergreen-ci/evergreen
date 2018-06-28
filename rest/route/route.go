package route

import (
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

// routeManagerFactory is a function type used to create RouteManagers and used to register handlders.
type routeManagerFactory func(string, int) *RouteManager

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
func (rm *RouteManager) Register(app *gimlet.APIApp, sc data.Connector) {
	for _, method := range rm.Methods {
		app.AddRoute(rm.Route).Version(rm.Version).Handler(makeHandler(method, sc)).Method(method.MethodType)
	}
}
