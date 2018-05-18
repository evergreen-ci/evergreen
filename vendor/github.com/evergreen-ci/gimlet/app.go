// Package gimlet is a toolkit for building JSON/HTTP interfaces (e.g. REST).
//
// Gimlet builds on standard library and common tools for building web
// applciations (e.g. Negroni and gorilla,) and is only concerned with
// JSON/HTTP interfaces, and omits support for aspects of HTTP
// applications outside of the scope of JSON APIs (e.g. templating,
// sessions.) Gimilet attempts to provide minimal convinences on top
// of great infrastucture so that your application can omit
// boilerplate and you don't have to build potentially redundant
// infrastructure.
package gimlet

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/phyber/negroni-gzip/gzip"
	"github.com/urfave/negroni"
)

// APIApp is a structure representing a single API service.
type APIApp struct {
	StrictSlash    bool
	isResolved     bool
	prefix         string
	defaultVersion int
	port           int
	router         *mux.Router
	address        string
	subApps        []*APIApp
	routes         []*APIRoute
	middleware     []Middleware
}

// NewApp returns a pointer to an application instance. These
// instances have reasonable defaults and include middleware to:
// recover from panics in handlers, log information about the request,
// and gzip compress all data. Users must specify a default version
// for new methods.
func NewApp() *APIApp {
	a := &APIApp{
		StrictSlash:    true,
		defaultVersion: -1, // this is the same as having no version prepended to the path.
		port:           3000,
	}

	a.AddMiddleware(negroni.NewRecovery())
	a.AddMiddleware(NewAppLogger())
	a.AddMiddleware(gzip.Gzip(gzip.DefaultCompression))

	return a
}

// SetDefaultVersion allows you to specify a default version for the
// application. Default versions must be 0 (no version,) or larger.
func (a *APIApp) SetDefaultVersion(version int) {
	if version < 0 {
		grip.Warningf("%d is not a valid version", version)
	} else {
		a.defaultVersion = version
		grip.Noticef("Set default api version to /v%d/", version)
	}
}

// Router is the getter for an APIApp's router object. If thetr
// application isn't resolved, then the error return value is non-nil.
func (a *APIApp) Router() (*mux.Router, error) {
	if a.isResolved {
		return a.router, nil
	}
	return nil, errors.New("application is not resolved")
}

// AddApp allows you to combine App instances, by taking one app and
// add its routes to the current app. Returns a non-nill error value
// if the current app is resolved. If the apps have different default
// versions set, the versions on the second app are explicitly set.
func (a *APIApp) AddApp(app *APIApp) error {
	// if we've already resolved then it has to be an error
	if a.isResolved {
		return errors.New("cannot merge an app into a resolved app")
	}

	a.subApps = append(a.subApps, app)
	return nil
}

// AddMiddleware adds a negroni handler as middleware to the end of
// the current list of middleware handlers.
func (a *APIApp) AddMiddleware(m Middleware) {
	a.middleware = append(a.middleware, m)
}

// Resolve processes the data in an application instance, including
// all routes and creats a mux.Router object for the application
// instance.
func (a *APIApp) Resolve() error {
	catcher := grip.NewCatcher()

	a.router = mux.NewRouter().StrictSlash(a.StrictSlash)

	for _, route := range a.routes {
		if !route.IsValid() {
			catcher.Add(fmt.Errorf("%d is an invalid api version. not adding route for %s",
				route.version, route.route))
			continue
		}

		var methods []string
		for _, m := range route.methods {
			methods = append(methods, strings.ToLower(m.String()))
		}

		if route.version > 0 {
			versionedRoute := getVersionedRoute(a.prefix, route.version, route.route)
			a.router.HandleFunc(versionedRoute, route.handler).Methods(methods...)
			grip.Debugln("added route for:", versionedRoute)
		}

		if route.version == a.defaultVersion {
			route.route = getDefaultRoute(a.prefix, route.route)
			a.router.HandleFunc(route.route, route.handler).Methods(methods...)
			grip.Debugln("added route for:", route.route)
		}
	}

	a.isResolved = true

	return catcher.Resolve()
}

func getVersionedRoute(prefix string, version int, route string) string {
	if strings.HasPrefix(route, prefix) {
		if prefix == "" {
			return fmt.Sprintf("/v%d%s", version, route)
		}
		route = route[len(prefix):]
	}

	return fmt.Sprintf("%s/v%d%s", prefix, version, route)
}

func getDefaultRoute(prefix, route string) string {
	if strings.HasPrefix(route, prefix) {
		return route
	}
	return prefix + route
}

// ResetMiddleware removes *all* middleware handlers from the current
// application.
func (a *APIApp) ResetMiddleware() {
	a.middleware = []Middleware{}
}

// getHander internal helper resolves the negorni middleware for the
// application and returns it in the form of a http.Handler for use in
// stitching together applicationstr
func (a *APIApp) getNegroni() (*negroni.Negroni, error) {
	if err := a.Resolve(); err != nil {
		return nil, err
	}

	catcher := grip.NewCatcher()
	n := negroni.New()
	for _, m := range a.middleware {
		n.Use(m)
	}
	n.UseHandler(a.router)
	for _, app := range a.subApps {
		if !strings.HasPrefix(app.prefix, a.prefix) {
			app.prefix = a.prefix + app.prefix
		}
		catcher.Add(app.Resolve())

		subNegroni, err := app.getNegroni()
		if err != nil {
			catcher.Add(err)
		}

		n.UseHandler(subNegroni)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return n, nil
}

// Handler returns a handler interface for integration with other
// server frameworks.
func (a *APIApp) Handler() (http.Handler, error) {
	return a.getNegroni()
}

// Run configured API service on the configured port. Before running
// the application, Run also resolves any sub-apps, and adds all
// routes.
func (a *APIApp) Run(ctx context.Context) error {
	n, err := a.getNegroni()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", a.address, a.port),
		Handler:           n,
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      time.Minute,
	}

	catcher := grip.NewBasicCatcher()
	serviceWait := make(chan struct{})
	go func() {
		defer recovery.LogStackTraceAndContinue("app service")

		grip.Noticef("starting app on: %s:$d", a.address, a.port)
		catcher.Add(srv.ListenAndServe())
	}()

	go func() {
		defer recovery.LogStackTraceAndContinue("server shutdown")
		catcher.Add(srv.Shutdown(ctx))
		close(serviceWait)
	}()

	<-serviceWait

	return catcher.Resolve()
}

// SetPort allows users to configure a default port for the API
// service. Defaults to 3000, and return errors will refuse to set the
// port to something unreasonable.
func (a *APIApp) SetPort(port int) error {
	defaultPort := 3000

	if port == a.port {
		grip.Warningf("port is already set to %d", a.port)
	} else if port <= 0 {
		a.port = defaultPort
		return fmt.Errorf("%d is not a valid port numbaer, using %d", port, defaultPort)
	} else if port > 65535 {
		a.port = defaultPort
		return fmt.Errorf("port %d is too large, using default port (%d)", port, defaultPort)
	} else if port < 1024 {
		a.port = defaultPort
		return fmt.Errorf("port %d is too small, using default port (%d)", port, defaultPort)
	} else {
		a.port = port
	}

	return nil
}

// SetHost sets the hostname or address for the application to listen
// on. Errors after resolving the application. You do not need to set
// this, and if unset the application will listen on the specified
// port on all interfaces.
func (a *APIApp) SetHost(name string) error {
	if a.isResolved {
		return fmt.Errorf("cannot set host to '%s', after resolving. Host is still '%s'",
			name, a.address)
	}

	a.address = name

	return nil
}

// SetPrefix sets the route prefix, adding a leading slash, "/", if
// neccessary.
func (a *APIApp) SetPrefix(p string) {
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	a.prefix = p
}
