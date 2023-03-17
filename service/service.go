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
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
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
		"agent":   evergreen.AgentVersion,
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
	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	app.AddMiddleware(gimlet.UserMiddleware(uis.env.UserManager(), uis.umconf))
	app.AddMiddleware(gimlet.NewAuthenticationHandler(gimlet.NewBasicAuthenticator(nil, nil), uis.env.UserManager()))
	app.AddMiddleware(gimlet.NewStatic("", http.Dir(filepath.Join(uis.Home, "public"))))

	clients := gimlet.NewApp()
	clients.AddMiddleware(gimlet.NewGzipDefault())
	clients.AddMiddleware(gimlet.NewStatic("/clients", http.Dir(filepath.Join(uis.Home, evergreen.ClientDirectory))))

	// in the future, we'll make the gimlet app here, but we
	// need/want to access and construct it separately.
	rest := GetRESTv1App(as)

	opts := route.HandlerOpts{
		APIQueue:            as.queue,
		URL:                 as.Settings.Ui.Url,
		GithubSecret:        []byte(as.Settings.Api.GithubWebhookSecret),
		TaskDispatcher:      as.taskDispatcher,
		TaskAliasDispatcher: as.taskAliasDispatcher,
	}
	route.AttachHandler(rest, opts)

	// Historically all rest interfaces were available in the API
	// and UI endpoints. While there were no users of restv1 in
	// with the "api" prefix, there are many users of restv2, so
	// we will continue to publish these routes in these
	// endpoints.
	apiRestV2 := gimlet.NewApp()
	apiRestV2.SetPrefix(evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix)
	opts = route.HandlerOpts{
		APIQueue:            as.queue,
		URL:                 as.Settings.Ui.Url,
		GithubSecret:        []byte(as.Settings.Api.GithubWebhookSecret),
		TaskDispatcher:      as.taskDispatcher,
		TaskAliasDispatcher: as.taskAliasDispatcher,
	}
	route.AttachHandler(apiRestV2, opts)

	// in the future the following functions will be above this
	// point, and we'll just have the app, but during the legacy
	// transition, we convert the app to a router and then attach
	// legacy routes directly.

	uiService := uis.GetServiceApp()
	apiService := as.GetServiceApp()

	router := mux.NewRouter().UseEncodedPath()
	router.Use(otelmux.Middleware("evergreen"))

	// the order that we merge handlers matters here, and we must
	// define more specific routes before less specific routes.
	return gimlet.MergeApplications(router, app, clients, uiService, rest, apiRestV2, apiService)
}
