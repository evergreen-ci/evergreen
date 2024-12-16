package service

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
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

func GetRouter(ctx context.Context, as *APIServer, uis *UIServer) (http.Handler, error) {
	app := gimlet.NewApp()
	app.AddMiddleware(gimlet.MakeRecoveryLogger())
	app.AddMiddleware(gimlet.UserMiddleware(ctx, uis.env.UserManager(), uis.umconf))
	app.AddMiddleware(gimlet.NewAuthenticationHandler(gimlet.NewBasicAuthenticator(nil, nil), uis.env.UserManager()))
	app.AddMiddleware(gimlet.NewStaticAuth("", http.Dir(filepath.Join(uis.Home, "public"))))

	clients := gimlet.NewApp()
	if uis.env.ClientConfig().S3URLPrefix != "" {
		clients.NoVersions = true
		clients.AddPrefixRoute("/clients").Get().Head().Wrap(gimlet.NewRequireAuthHandler()).Handler(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/clients")
			path = uis.env.ClientConfig().S3URLPrefix + path
			http.Redirect(w, r, path, http.StatusTemporaryRedirect)
		})
	}

	// in the future, we'll make the gimlet app here, but we
	// need/want to access and construct it separately.
	rest := GetRESTv1App(as)

	opts := route.HandlerOpts{
		APIQueue:            as.queue,
		URL:                 as.Settings.Ui.Url,
		GithubSecret:        []byte(as.Settings.GithubWebhookSecret),
		TaskDispatcher:      as.taskDispatcher,
		TaskAliasDispatcher: as.taskAliasDispatcher,
	}
	route.AttachHandler(rest, opts)

	// Historically all rest interfaces were available in the API
	// and UI endpoints. While there were no users of restv1 in
	// with the "api" prefix, there are many users of restv2, so
	// we will continue to publish these routes in these
	// endpoints.
	//
	//	@title						Evergreen REST v2 API
	//	@host						evergreen.mongodb.com
	//	@BasePath					/rest/v2
	//	@accept						json
	//	@produce					json
	//	@schemes					https
	//	@externalDocs.description	Click here for information on authentication, pagination, and other details.
	//	@externalDocs.url			https://docs.devprod.prod.corp.mongodb.com/evergreen/API/REST-V2-Usage/
	//
	//	@tag.name					admin
	//	@tag.description			Admins are users with high-level permissions within Evergreen.
	//
	//	@tag.name					annotations
	//	@tag.description			Annotations are added by users, usually programmatically, to track known and suspected causes of task failures.
	//
	//	@tag.name					builds
	//	@tag.description			A build is a set of tasks in a single build variant in a single version.
	//
	//	@tag.name					distros
	//	@tag.description			A distro is a set of hosts that runs tasks. A static distro is configured with a list of IP addresses, while a dynamic distro scales with demand.
	//
	//	@tag.name					hosts
	//	@tag.description			A host is an Evergrene-managed EC2 VM, static host, or container.
	//
	//	@tag.name					info
	//	@tag.description			Info is general information about the system.
	//
	//	@tag.name					keys
	//	@tag.description			Keys are SSH keys for users to SSH into spawn hosts.
	//
	//	@tag.name					manifests
	//	@tag.description			A manifest tracks metadata about a version.
	//
	//	@tag.name					patches
	//	@tag.description			A patch build is a version not triggered by a commit to a repository. It either runs tasks on a base commit plus some diff if submitted by the CLI or on a git branch if created by a GitHub pull request.
	//
	//	@tag.name					projects
	//	@tag.description			A project tracks a GitHub repository.
	//
	//	@tag.name					select
	//	@tag.description			The select endpoints return a subset of tests to run for a given task.
	//
	//	@tag.name					tasks
	//	@tag.description			The fundamental unit of execution is the task. A task corresponds to a box on the waterfall page.
	//
	//	@tag.name					tests
	//	@tag.description			A test is sent to Evergreen in a known format by a command during a task, parsed by Evergreen, and displayed on the task page.
	//
	//	@tag.name					users
	//	@tag.description			A user is an Evergreen user.
	//
	//	@tag.name					versions
	//	@tag.description			A version, which corresponds to a vertical slice of tasks on the waterfall, is all tasks for a given commit or patch build.
	//
	//	@securitydefinitions.apikey	Api-User
	//	@in							header
	//	@name						Api-User
	//	@description				the `user` field from https://spruce.mongodb.com/preferences/cli
	//
	//	@securitydefinitions.apikey	Api-Key
	//	@in							header
	//	@name						Api-Key
	//	@description				the `api-key` field from https://spruce.mongodb.com/preferences/cli
	apiRestV2 := gimlet.NewApp()
	apiRestV2.SetPrefix(evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix)
	opts = route.HandlerOpts{
		APIQueue:            as.queue,
		URL:                 as.Settings.Ui.Url,
		GithubSecret:        []byte(as.Settings.GithubWebhookSecret),
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
	return gimlet.MergeApplicationsWithRouter(router, app, clients, uiService, rest, apiRestV2, apiService)
}
