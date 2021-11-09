package gimlet

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/urfave/negroni"
)

// AssembleHandler takes a router and one or more applications and
// returns an application.
//
// Eventually the router will become an implementation detail of
// this/related functions.
func AssembleHandler(router *mux.Router, apps ...*APIApp) (http.Handler, error) {
	catcher := grip.NewBasicCatcher()
	mws := []Middleware{}

	seenPrefixes := make(map[string]struct{})

	for _, app := range apps {
		if app.prefix != "" {
			if _, ok := seenPrefixes[app.prefix]; ok {
				catcher.Add(errors.Errorf("route prefix '%s' defined more than once", app.prefix))
			}
			seenPrefixes[app.prefix] = struct{}{}

			n := negroni.New()
			for _, m := range app.middleware {
				n.Use(m)
			}

			r := router.PathPrefix(app.prefix).Subrouter()
			catcher.Add(app.attachRoutes(r, false)) // this adds wrapper middlware
			n.UseHandler(r)
			router.PathPrefix(app.prefix).Handler(n)
		} else {
			mws = append(mws, app.middleware...)

			catcher.Add(app.attachRoutes(router, true))
		}
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	n := negroni.New()
	for _, m := range mws {
		n.Use(m)
	}
	n.UseHandler(router)

	return n, nil
}

// MergeApplications takes a number of gimlet applications and
// resolves them, returning an http.Handler.
func MergeApplications(apps ...*APIApp) (http.Handler, error) {
	if len(apps) == 0 {
		return nil, errors.New("must specify at least one application")
	}

	return AssembleHandler(mux.NewRouter(), apps...)
}

// Merge takes multiple application instances and merges all of their
// routes into a single application.
//
// You must only call Merge once per base application, and you must
// pass more than one or more application to merge. Additionally, it
// is not possible to merge applications into a base application that
// has a prefix specified.
//
// When the merging application does not have a prefix, the merge
// operation will error if you attempt to merge applications that have
// duplicate cases. Similarly, you cannot merge multiple applications
// that have the same prefix: you should treat these errors as fatal.
func (a *APIApp) Merge(apps ...*APIApp) error {
	if a.prefix != "" {
		return errors.New("cannot merge applications into an application with a prefix")
	}

	if apps == nil {
		return errors.New("must specify apps to merge")
	}

	if a.hasMerged {
		return errors.New("can only call merge once per root application")
	}

	catcher := grip.NewBasicCatcher()
	seenPrefixes := make(map[string]struct{})

	for _, app := range apps {
		if app.prefix != "" {
			if _, ok := seenPrefixes[app.prefix]; ok {
				catcher.Errorf("route prefix '%s' defined more than once", app.prefix)
			}
			seenPrefixes[app.prefix] = struct{}{}

			for _, route := range app.routes {
				r := a.PrefixRoute(app.prefix).Route(route.route).Version(route.version).Handler(route.handler)
				for _, m := range route.methods {
					r = r.Method(m.String())
				}

				r.overrideAppPrefix = route.overrideAppPrefix
				r.wrappers = append(app.middleware, route.wrappers...)
			}
		} else if app.middleware == nil {
			for _, r := range app.routes {
				if a.containsRoute(r.route, r.version, r.methods) {
					catcher.Errorf("cannot merge route '%s' with existing application that already has this route defined", r.route)
				}
			}

			a.routes = append(a.routes, app.routes...)
		} else {
			for _, route := range app.routes {
				if a.containsRoute(route.route, route.version, route.methods) {
					catcher.Errorf("cannot merge route '%s' with existing application that already has this route defined", route.route)
				}

				r := a.Route().Route(route.route).Version(route.version)
				for _, m := range route.methods {
					r = r.Method(m.String())
				}
				r.overrideAppPrefix = route.overrideAppPrefix
				r.wrappers = append(app.middleware, route.wrappers...)
			}
		}
	}

	a.hasMerged = true

	return catcher.Resolve()
}

func (a *APIApp) containsRoute(path string, version int, methods []httpMethod) bool {
	for _, r := range a.routes {
		if r.route == path && r.version == version {
			for _, m := range r.methods {
				for _, rm := range methods {
					if m == rm {
						return true
					}
				}
			}
		}
	}

	return false
}
