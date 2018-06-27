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
	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	app.AddMiddleware(gimlet.UserMiddleware(uis.UserManager, GetUserMiddlewareConf()))
	app.AddMiddleware(negroni.NewStatic(http.Dir(filepath.Join(uis.Home, "public"))))

	restv1 := GetRESTv1App(as)

	// in the future, we'll make the gimlet app here, but we
	// need/want to access and construct it separately.

	restv2 := gimlet.NewApp()
	restv2.ResetMiddleware()
	restv2.SetPrefix(evergreen.RestRoutePrefix)
	route.AttachHandler(restv2, as.queue, as.Settings.Ui.Url, as.Settings.SuperUsers, []byte(as.Settings.Api.GithubWebhookSecret))

	// Historically all rest interfaces were available in the API
	// and UI endpoints. While there were no users of restv1 in
	// with the "api" prefix, there are many users of restv2, so
	// we will continue to publish these routes in these
	// endpoints.
	apiRestv2 := gimlet.NewApp()
	apiRestv2.ResetMiddleware()
	apiRestv2.SetPrefix(evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix)
	route.AttachHandler(apiRestv2, as.queue, as.Settings.Ui.Url, as.Settings.SuperUsers, []byte(as.Settings.Api.GithubWebhookSecret))

	// in the future the following functions will be above this
	// point, and we'll just have the app, but during the legacy
	// transition, we convert the app to a router and then attach
	// legacy routes directly.
	r := mux.NewRouter()

	uis.AttachRoutes(r)
	as.AttachRoutes(r)

	return gimlet.AssembleHandler(r, app, restv1, restv2, apiRestv2)
}
