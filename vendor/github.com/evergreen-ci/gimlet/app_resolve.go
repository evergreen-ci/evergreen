package gimlet

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/urfave/negroni"
)

// Handler returns a handler interface for integration with other
// server frameworks.
func (a *APIApp) Handler() (http.Handler, error) {
	return a.getNegroni()
}

// Resolve processes the data in an application instance, including
// all routes and creats a mux.Router object for the application
// instance.
func (a *APIApp) Resolve() error {
	if a.isResolved {
		return nil
	}

	if a.router == nil {
		a.router = mux.NewRouter().StrictSlash(a.StrictSlash)
	}

	if err := a.attachRoutes(a.router, true); err != nil {
		return err
	}

	a.isResolved = true

	return nil
}

// getHander internal helper resolves the negorni middleware for the
// application and returns it in the form of a http.Handler for use in
// stitching together applications.
func (a *APIApp) getNegroni() (*negroni.Negroni, error) {
	if err := a.Resolve(); err != nil {
		return nil, err
	}

	n := negroni.New()
	for _, m := range a.middleware {
		n.Use(m)
	}
	n.UseHandler(a.router)

	return n, nil
}

func (a *APIApp) attachRoutes(router *mux.Router, addPrefix bool) error {
	catcher := grip.NewCatcher()
	for _, route := range a.routes {
		if !route.IsValid() {
			catcher.Add(fmt.Errorf("%s is not a valid route, skipping", route.route))
			continue
		}

		var methods []string
		for _, m := range route.methods {
			methods = append(methods, strings.ToLower(m.String()))
		}

		handler := getRouteHandlerWithMiddlware(a.wrappers, route.handler)
		if a.NoVersions {
			router.Handle(route.resolveLegacyRoute(a, addPrefix), handler)
		} else if route.version >= 0 {
			versionedRoute := route.resolveVersionedRoute(a, addPrefix)
			router.Handle(versionedRoute, handler).Methods(methods...)
		} else {
			catcher.Add(fmt.Errorf("skipping '%s', because of versioning error", route))
		}
	}

	return catcher.Resolve()
}

func (r *APIRoute) getRoutePrefix(app *APIApp, addPrefix bool) string {
	if !addPrefix {
		return ""
	}

	if r.prefix != "" {
		return r.prefix
	}

	return app.prefix
}

func (r *APIRoute) resolveLegacyRoute(app *APIApp, addPrefix bool) string {
	prefix := r.getRoutePrefix(app, addPrefix)

	if prefix == "" {
		return r.route
	}

	if strings.HasPrefix(r.route, "/") {
		return prefix + r.route
	}

	return strings.Join([]string{prefix, r.route}, "/")
}

func (r *APIRoute) resolveVersionedRoute(app *APIApp, addPrefix bool) string {
	var (
		versionPrefix string
		prefix        string
		route         string
	)

	if !app.SimpleVersions {
		versionPrefix = "v"
	}

	prefix = r.getRoutePrefix(app, addPrefix)
	if strings.HasPrefix(r.route, prefix) {
		if prefix == "" {
			return fmt.Sprintf("/%s%d%s", versionPrefix, r.version, r.route)
		}
		route = r.route[len(prefix):]
	} else {
		route = r.route
	}

	return fmt.Sprintf("%s/%s%d%s", prefix, versionPrefix, r.version, route)
}

func getRouteHandlerWithMiddlware(mws []Middleware, route http.Handler) http.Handler {
	if len(mws) == 0 {
		return route
	}

	n := negroni.New()
	for _, m := range mws {
		n.Use(m)
	}
	n.UseHandler(route)
	return n
}
