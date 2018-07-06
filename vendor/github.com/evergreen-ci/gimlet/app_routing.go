package gimlet

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mongodb/grip"
)

// APIRoute is a object that represents each route in the application
// and includes the route and associate internal metadata for the
// route.
type APIRoute struct {
	route             string
	prefix            string
	methods           []httpMethod
	handler           http.HandlerFunc
	wrappers          []Middleware
	version           int
	overrideAppPrefix bool
}

func (r *APIRoute) String() string {
	var methods []string
	for _, m := range r.methods {
		methods = append(methods, m.String())
	}

	return fmt.Sprintf(
		"r='%s%s', v='%d', methods=[%s], defined=%t, prefixOverride=%t",
		r.prefix,
		r.route,
		r.version,
		strings.Join(methods, ", "),
		r.handler != nil,
		r.overrideAppPrefix,
	)
}

// AddRoute is the primary method for creating and registering a new route with an
// application. Use as the root of a method chain, passing this method
// the path of the route.
func (a *APIApp) AddRoute(r string) *APIRoute {
	route := &APIRoute{route: r, version: -1}

	// data validation and cleanup
	if !strings.HasPrefix(route.route, "/") {
		route.route = "/" + route.route
	}

	a.routes = append(a.routes, route)

	return route
}

// Route returns a route object without a route name configured.
func (a *APIApp) Route() *APIRoute {
	route := &APIRoute{version: -1}

	a.routes = append(a.routes, route)

	return route
}

// PrefixRoute allows you to create a new route with a
// prefix. Prefixes override the applications global prefix.
func (a *APIApp) PrefixRoute(p string) *APIRoute {
	route := &APIRoute{prefix: p, version: -1}

	if !strings.HasPrefix(route.prefix, "/") {
		route.prefix = "/" + route.prefix
	}

	a.routes = append(a.routes, route)

	return route
}

// Route allows you to set or reset the route path on an existing route.
func (r *APIRoute) Route(route string) *APIRoute {
	r.route = route

	if !strings.HasPrefix(r.route, "/") {
		r.route = "/" + r.route
	}

	return r
}

// IsValid checks if a route has is valid and populated.
func (r *APIRoute) IsValid() bool {
	switch {
	case len(r.methods) == 0:
		return false
	case r.handler == nil:
		return false
	case r.route == "":
		return false
	default:
		return true
	}
}

// ClearWrappers resets the routes middlware wrappers.
func (r *APIRoute) ClearWrappers() { r.wrappers = []Middleware{} }

// Wrap adds a middleware that is applied specifically to this
// route. Route-specific middlware is applied after application specific
// middleware (when there's a route or application prefix) and before
// global application middleware (when merging applications without prefixes.)
func (r *APIRoute) Wrap(m ...Middleware) *APIRoute { r.wrappers = append(r.wrappers, m...); return r }

// Prefix allows per-route prefixes, which will override the application's global prefix if set.
func (r *APIRoute) Prefix(p string) *APIRoute {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	r.prefix = p
	return r
}

// OverridePrefix forces the route's prefix to override the global app
// prefix. By default, the route's prefix is combined (after a
// version) with the application's prefix.
//
// When setting override prefix on an application that has a prefix
// specified, this option will not take effect in cases where you're
// merging multiple applications together.
func (r *APIRoute) OverridePrefix() *APIRoute { r.overrideAppPrefix = true; return r }

// Version allows you to specify an integer for the version of this
// route. Version is chainable.
func (r *APIRoute) Version(version int) *APIRoute {
	if version < 0 {
		grip.Warningf("%d is not a valid version", version)
	}

	r.version = version
	return r
}

// Handler makes it possible to register an http.HandlerFunc with a
// route. Chainable. The common pattern for implementing these
// functions is to write functions and methods in your application
// that *return* handler fucntions, so you can pass application state
// or other data into to the handlers when the applications start,
// without relying on either global state *or* running into complex
// typing issues.
func (r *APIRoute) Handler(h http.HandlerFunc) *APIRoute {
	if r.handler != nil {
		grip.Warningf("called Handler more than once for route %s", r.route)
	} else if h == nil {
		grip.Alertf("adding nil route handler will prorobably result in runtime panics for '%s'", r.route)
	}

	r.handler = h

	return r
}

// RouteHandler defines a handler defined using the RouteHandler
// interface, which provides additional infrastructure for defining
// handlers, to separate input parsing, business logic, and response
// generation.
func (r *APIRoute) RouteHandler(h RouteHandler) *APIRoute {
	if r.handler != nil {
		grip.Warningf("called Handler more than once for route %s", r.route)
	} else if h == nil {
		grip.Alertf("adding nil route handler will prorobably result in runtime panics for '%s'", r.route)
	}

	r.handler = handleHandler(h)

	return r
}

// Get is a chainable method to add a handler for the GET method to
// the current route. Routes may specify multiple methods.
func (r *APIRoute) Get() *APIRoute {
	r.methods = append(r.methods, get)
	return r
}

// Put is a chainable method to add a handler for the PUT method to
// the current route. Routes may specify multiple methods.
func (r *APIRoute) Put() *APIRoute {
	r.methods = append(r.methods, put)
	return r
}

// Post is a chainable method to add a handler for the POST method to
// the current route. Routes may specify multiple methods.
func (r *APIRoute) Post() *APIRoute {
	r.methods = append(r.methods, post)
	return r
}

// Delete is a chainable method to add a handler for the DELETE method
// to the current route. Routes may specify multiple methods.
func (r *APIRoute) Delete() *APIRoute {
	r.methods = append(r.methods, delete)
	return r
}

// Patch is a chainable method to add a handler for the PATCH method
// to the current route. Routes may specify multiple methods.
func (r *APIRoute) Patch() *APIRoute {
	r.methods = append(r.methods, patch)
	return r
}

// Head is a chainable method to add a handler for the HEAD method
// to the current route. Routes may specify multiple methods.
func (r *APIRoute) Head() *APIRoute {
	r.methods = append(r.methods, head)
	return r
}

// Method makes it possible to specify an HTTP method pragmatically.
func (r *APIRoute) Method(m string) *APIRoute {
	switch m {
	case get.String():
		return r.Get()
	case put.String():
		return r.Put()
	case post.String():
		return r.Post()
	case delete.String():
		return r.Delete()
	case patch.String():
		return r.Patch()
	case head.String():
		return r.Head()
	default:
		return r
	}
}
