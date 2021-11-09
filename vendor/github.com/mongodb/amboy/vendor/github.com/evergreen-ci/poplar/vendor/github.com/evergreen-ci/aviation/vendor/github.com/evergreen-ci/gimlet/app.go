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
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// WaitFunc is a function type returned by some functions that allows
// callers to wait on background processes started by the returning
// function.
//
// If the context passed to a wait function is canceled then the wait
// function should return immediately. You may wish to pass contexts
// with a different timeout to the wait function from the one you
// passed to the outer function to ensure correct waiting semantics.
type WaitFunc func(context.Context)

// APIApp is a structure representing a single API service.
type APIApp struct {
	StrictSlash    bool
	SimpleVersions bool
	NoVersions     bool
	isResolved     bool
	hasMerged      bool
	prefix         string
	port           int
	router         *mux.Router
	address        string
	routes         []*APIRoute
	middleware     []Middleware
	wrappers       []Middleware
}

// NewApp returns a pointer to an application instance. These
// instances have reasonable defaults and include middleware to:
// recover from panics in handlers, log information about the request,
// and gzip compress all data. Users must specify a default version
// for new methods.
func NewApp() *APIApp {
	a := &APIApp{
		port:        3000,
		StrictSlash: true,
	}

	return a
}

// Router is the getter for an APIApp's router object. If thetr
// application isn't resolved, then the error return value is non-nil.
func (a *APIApp) Router() (*mux.Router, error) {
	if a.isResolved {
		return a.router, nil
	}
	return nil, errors.New("application is not resolved")
}

// AddMiddleware adds a negroni handler as middleware to the end of
// the current list of middleware handlers.
//
// All Middleware is added before the router. If your middleware
// depends on executing within the context of the router/muxer, add it
// as a wrapper.
func (a *APIApp) AddMiddleware(m Middleware) {
	a.middleware = append(a.middleware, m)
}

// AddWrapper adds a negroni handler as a wrapper for a specific route.
//
// These wrappers execute in the context of the router/muxer. If your
// middleware does not need access to the muxer's state, add it as a
// middleware.
func (a *APIApp) AddWrapper(m Middleware) {
	a.wrappers = append(a.wrappers, m)
}

// ResetMiddleware removes *all* middleware handlers from the current
// application.
func (a *APIApp) ResetMiddleware() {
	a.middleware = []Middleware{}
}

// RestWrappers removes all route-specific middleware from the
// current application.
func (a *APIApp) RestWrappers() {
	a.wrappers = []Middleware{}
}

// Run configured API service on the configured port. Before running
// the application, Run also resolves any sub-apps, and adds all
// routes.
//
// If you cancel the context that you pass to run, the application
// will gracefully shutdown, and wait indefinitely until the
// application has returned. To get different waiting behavior use
// BackgroundRun.
func (a *APIApp) Run(ctx context.Context) error {
	wait, err := a.BackgroundRun(ctx)

	if err != nil {
		return errors.WithStack(err)
	}

	wait(context.Background())

	return nil
}

// BackgroundRun is a non-blocking form of Run that allows you to
// manage a service running in the background.
func (a *APIApp) BackgroundRun(ctx context.Context) (WaitFunc, error) {
	n, err := a.getNegroni()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	conf := ServerConfig{
		Handler: n,
		Address: fmt.Sprintf("%s:%d", a.address, a.port),
		Timeout: time.Minute,
		Info:    fmt.Sprintf("app with '%s' prefix", a.prefix),
	}

	srv, err := conf.Resolve()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	grip.Noticef("starting %s on: %s:%d", a.prefix, a.address, a.port)

	return srv.Run(ctx)
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
