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

func (a *APIApp) attachRoutes(router *mux.Router, addAppPrefix bool) error {
	router.StrictSlash(a.StrictSlash)
	catcher := grip.NewCatcher()
	for _, route := range a.routes {
		if !route.IsValid() {
			catcher.Add(fmt.Errorf("%s is not a valid route, skipping", route))
			continue
		}

		var methods []string
		for _, m := range route.methods {
			methods = append(methods, strings.ToLower(m.String()))
		}

		handler := route.getHandlerWithMiddlware(a.wrappers)

		if route.version >= 0 {
			versionedRoute := route.resolveVersionedRoute(a, addAppPrefix)
			router.Handle(versionedRoute, handler).Methods(methods...)
		} else if a.NoVersions {
			router.Handle(route.resolveLegacyRoute(a, addAppPrefix), handler).Methods(methods...)
		} else {
			catcher.Add(fmt.Errorf("skipping '%s', because of versioning error", route))
		}
	}

	return catcher.Resolve()
}

func (r *APIRoute) getRoutePrefix(app *APIApp, addAppPrefix bool) string {
	if !addAppPrefix {
		return ""
	}

	if r.overrideAppPrefix && r.prefix != "" {
		return r.prefix
	}

	return app.prefix
}

func (r *APIRoute) resolveLegacyRoute(app *APIApp, addAppPrefix bool) string {
	var output string

	prefix := r.getRoutePrefix(app, addAppPrefix)

	if prefix != "" {
		output += prefix
	}

	if r.prefix != prefix && r.prefix != "" {
		output += r.prefix
	}

	output += r.route

	return output
}

func (r *APIRoute) getVersionPart(app *APIApp) string {
	var versionPrefix string

	if !app.SimpleVersions {
		versionPrefix = "v"
	}

	return fmt.Sprintf("/%s%d", versionPrefix, r.version)
}

func (r *APIRoute) resolveVersionedRoute(app *APIApp, addAppPrefix bool) string {
	var (
		output string
		route  string
	)

	route = r.route
	firstPrefix := r.getRoutePrefix(app, addAppPrefix)

	if firstPrefix != "" {
		output += firstPrefix
	}

	output += r.getVersionPart(app)

	if r.prefix != firstPrefix && r.prefix != "" {
		output += r.prefix
	}

	output += route

	return output
}

func (r *APIRoute) getHandlerWithMiddlware(mws []Middleware) http.Handler {
	if len(mws) == 0 && len(r.wrappers) == 0 {
		return r.handler
	}

	n := negroni.New()
	for _, m := range mws {
		n.Use(m)
	}
	for _, m := range r.wrappers {
		n.Use(m)
	}
	n.UseHandler(r.handler)
	return n
}
