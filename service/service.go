package service

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/urfave/negroni"
)

const (
	WebRootPath  = "service"
	Templates    = "templates"
	Static       = "static"
	DefaultSkip  = 0
	DefaultLimit = 10
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
	// in the future, we'll make the gimlet app here, but we
	// need/want to access and construct it separately.
	app, err := GetRESTv1App(as, uis.UserManager)
	if err != nil {
		return nil, err
	}

	APIV2Prefix := evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix
	route.AttachHandler(app, as.queue, as.Settings.ApiUrl, APIV2Prefix, as.Settings.SuperUsers, []byte(as.Settings.Api.GithubWebhookSecret))
	route.AttachHandler(app, uis.queue, uis.Settings.Ui.Url, evergreen.RestRoutePrefix, uis.Settings.SuperUsers, []byte(uis.Settings.Api.GithubWebhookSecret))

	app.AddMiddleware(negroni.NewStatic(http.Dir(filepath.Join(uis.Home, "public"))))

	if err = app.Resolve(); err != nil {
		return nil, err
	}
	// in the future the following functions will be above this
	// point, and we'll just have the app, but during the legacy
	// transition, we convert the app to a router and then attach
	// legacy routes directly.

	router, err := app.Router()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = uis.AttachRoutes(router)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	as.AttachRoutes(router)

	return app.Handler()
}
