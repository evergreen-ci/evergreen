package service

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/mux"
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
	app := gimlet.NewApp()
	app.ResetMiddleware()
	app.AddMiddleware(NewRecoveryLogger())
	app.AddMiddleware(gimlet.UserMiddleware(uis.UserManager, GetUserMiddlewareConf()))

	// in the future, we'll make the gimlet app here, but we
	// need/want to access and construct it separately.
	restv1, err := GetRESTv1App(as)
	if err != nil {
		return nil, err
	}

	restv2API := gimlet.NewApp()
	restv2UI := gimlet.NewApp()
	restv2API.ResetMiddleware()
	restv2UI.ResetMiddleware()

	restv1.AddMiddleware(negroni.NewStatic(http.Dir(filepath.Join(uis.Home, "public"))))
	restv2API.SetPrefix(evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix)
	restv2UI.SetPrefix(evergreen.RestRoutePrefix)

	// attach restv2 handlers
	route.AttachHandler(restv2API, as.queue, as.Settings.ApiUrl, as.Settings.SuperUsers, []byte(as.Settings.Api.GithubWebhookSecret))
	route.AttachHandler(restv2UI, uis.queue, uis.Settings.Ui.Url, uis.Settings.SuperUsers, []byte(uis.Settings.Api.GithubWebhookSecret))

	// in the future the following functions will be above this
	// point, and we'll just have the app, but during the legacy
	// transition, we convert the app to a router and then attach
	// legacy routes directly.

	r := mux.NewRouter()
	err = uis.AttachRoutes(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	as.AttachRoutes(r)

	return gimlet.AssembleHandler(r, app, restv1, restv2API, restv2UI)
}
