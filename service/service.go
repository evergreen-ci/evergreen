package service

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/negroni"
)

// GetServer produces an HTTP server instance for a handler.
func GetServer(addr string, n http.Handler) *http.Server {
	grip.Notice(message.Fields{
		"action":  "starting service",
		"service": addr,
		"build":   evergreen.BuildRevision,
		"process": grip.Name(),
	})

	return &http.Server{
		Addr:              addr,
		Handler:           n,
		ReadTimeout:       time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      time.Minute,
	}
}

func GetRouter(as *APIServer, uis *UIServer) (http.Handler, error) {
	app := gimlet.NewApp()

	restv1, err := GetRESTv1App(as, as.UserManager)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	app.AddApp(restv1)

	// in the future the following functions will be above this
	// point, and we'll just have the app, but during the legacy
	// transition, we convert the app to a router and then attach
	// legacy routes directly.

	legacyApp := gimlet.NewApp()
	legacyApp.ResetMiddleware()
	legacyApp.AddMiddleware(NewRecoveryLogger())
	legacyApp.AddMiddleware(negroni.HandlerFunc(UserMiddleware(as.UserManager)))
	legacyApp.AddMiddleware(negroni.NewStatic(http.Dir(filepath.Join(uis.Home, "public"))))

	router, err := legacyApp.Router()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	as.AttachRoutes(router)
	err = uis.AttachRoutes(router)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	app.AddApp(legacyApp)

	return app.Handler()
}
