package gimlet

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/urfave/negroni"
)

func AssembleHandler(router *mux.Router, apps ...*APIApp) (http.Handler, error) {
	catcher := grip.NewBasicCatcher()
	mws := []Middleware{}

	for _, app := range apps {
		if app.prefix != "" {
			n := negroni.New()
			for _, m := range app.middleware {
				n.Use(m)
			}

			r := router.PathPrefix(app.prefix).Subrouter()
			catcher.Add(app.attachRoutes(r, false)) // this adds wrapper middlware
			n.UseHandler(r)
			router.PathPrefix(app.prefix).Handler(n)
		} else {
			for _, m := range app.middleware {
				mws = append(mws, m)
			}

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
