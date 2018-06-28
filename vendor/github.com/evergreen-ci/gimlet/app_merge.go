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

	if len(apps) == 1 {
		router.StrictSlash(apps[0].StrictSlash)
	}

	n := negroni.New()
	for _, m := range mws {
		n.Use(m)
	}
	n.UseHandler(router)

	return n, nil
}
