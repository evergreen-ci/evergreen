package service

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/PuerkitoBio/rehttp"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/csrf"
	"github.com/gorilla/handlers"
	"github.com/gorilla/sessions"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// UIServer provides a web interface for Evergreen.
type UIServer struct {
	// Home is the root path on disk from which relative urls are constructed for loading
	// plugins or other assets.
	Home string

	// The root URL of the server, used in redirects for instance.
	RootURL string

	umconf      gimlet.UserMiddlewareConfiguration
	Settings    evergreen.Settings
	CookieStore *sessions.CookieStore
	jiraHandler thirdparty.JiraHandler

	hostCache map[string]hostCacheItem

	queue amboy.Queue
	env   evergreen.Environment
}

const hostCacheTTL = 30 * time.Second

type hostCacheItem struct {
	dnsName              string
	inserted             time.Time
	owner                string
	isVirtualWorkstation bool
	IsCluster            bool
	isRunning            bool
}

func NewUIServer(env evergreen.Environment, queue amboy.Queue, home string) (*UIServer, error) {
	settings := env.Settings()

	cookieStore := sessions.NewCookieStore([]byte(settings.Ui.Secret))
	cookieStore.Options.HttpOnly = true
	cookieStore.Options.Secure = true

	uis := &UIServer{
		Settings:    *settings,
		env:         env,
		queue:       queue,
		Home:        home,
		CookieStore: cookieStore,
		jiraHandler: thirdparty.NewJiraHandler(*settings.Jira.Export()),
		umconf: gimlet.UserMiddlewareConfiguration{
			HeaderKeyName:                   evergreen.APIKeyHeader,
			HeaderUserName:                  evergreen.APIUserHeader,
			CookieName:                      evergreen.AuthTokenCookie,
			CookieTTL:                       365 * 24 * time.Hour,
			CookiePath:                      "/",
			CookieDomain:                    settings.Ui.LoginDomain,
			StaticKeysDisabledForHumanUsers: settings.ServiceFlags.StaticAPIKeysDisabled,
		},
		hostCache: make(map[string]hostCacheItem),
	}

	if settings.AuthConfig.Kanopy != nil {
		uis.umconf.OIDC = &gimlet.OIDCConfig{
			KeysetURL:  settings.AuthConfig.Kanopy.KeysetURL,
			Issuer:     settings.AuthConfig.Kanopy.Issuer,
			HeaderName: settings.AuthConfig.Kanopy.HeaderName,
			DisplayNameFromID: func(id string) string {
				return cases.Title(language.English).String(strings.Join(strings.Split(id, "."), " "))
			},
		}
	}

	if err := uis.umconf.Validate(); err != nil {
		return nil, errors.Wrap(err, "programmer error; invalid user middleware configuration")
	}

	return uis, nil
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (uis *UIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	if err == nil {
		return
	}

	grip.Error(message.WrapError(err, message.Fields{
		"method":  r.Method,
		"url":     r.URL,
		"code":    code,
		"request": gimlet.GetRequestID(r.Context()),
		"stack":   string(debug.Stack()),
	}))

	// if JSON is the preferred content type for the request, reply with a json message
	if strings.HasPrefix(r.Header.Get("accept"), "application/json") {
		gimlet.WriteJSONResponse(w, code, struct {
			Error string `json:"error"`
		}{err.Error()})
	} else {
		// Not a JSON request, so write plaintext.
		http.Error(w, err.Error(), code)
	}
}

// NewRouter sets up a request router for the UI, installing
// hard-coded routes as well as those belonging to plugins.
func (uis *UIServer) GetServiceApp() *gimlet.APIApp {
	needsLogin := gimlet.WrapperMiddleware(uis.requireLogin)
	needsLoginNoRedirect := gimlet.WrapperMiddleware(uis.requireLoginStatusUnauthorized)
	needsContext := gimlet.WrapperMiddleware(uis.loadCtx)
	allowsCORS := gimlet.WrapperMiddleware(uis.setCORSHeaders)
	wrapUserForMCP := gimlet.WrapperMiddleware(uis.wrapUserForMCP)
	ownsHost := gimlet.WrapperMiddleware(uis.ownsHost)
	vsCodeRunning := gimlet.WrapperMiddleware(uis.vsCodeRunning)
	viewTasks := route.RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	viewLogs := route.RequiresProjectPermission(evergreen.PermissionLogs, evergreen.LogsView)
	requireSage := route.NewSageMiddleware()

	app := gimlet.NewApp()
	app.NoVersions = true

	// User login and logout
	app.AddRoute("/login").Handler(uis.loginRedirect).Get()
	app.AddRoute("/login").Wrap(allowsCORS).Handler(uis.login).Post()
	app.AddRoute("/logout").Wrap(allowsCORS).Handler(uis.logout).Get()

	if h := uis.env.UserManager().GetLoginHandler(uis.RootURL); h != nil {
		app.AddRoute("/login/redirect").Handler(h).Get()
	}
	if h := uis.env.UserManager().GetLoginCallbackHandler(); h != nil {
		app.AddRoute("/login/redirect/callback").Handler(h).Get()
	}

	app.AddRoute("/robots.txt").Get().Wrap(needsLogin).Handler(func(rw http.ResponseWriter, r *http.Request) {
		_, err := rw.Write([]byte(strings.Join([]string{
			"User-agent: *",
			"Disallow: /",
		}, "\n")))
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(err))
		}
	})

	if uis.Settings.Ui.CsrfKey != "" {
		app.AddMiddleware(gimlet.WrapperHandlerMiddleware(
			csrf.Protect([]byte(uis.Settings.Ui.CsrfKey), csrf.ErrorHandler(http.HandlerFunc(ForbiddenHandler))),
		))
	}

	// GraphQL
	app.AddRoute("/graphql").Wrap(allowsCORS, needsLogin).Handler(playground.ApolloSandboxHandler("GraphQL playground", "/graphql/query")).Get()
	app.AddRoute("/graphql/query").
		Wrap(allowsCORS, needsLoginNoRedirect).
		Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.CompressHandler(http.HandlerFunc(graphql.Handler(uis.Settings.Api.URL, true))).ServeHTTP(w, r)
		})).
		Post().Get()

	// MCP-only GraphQL (queries only, no mutations)
	app.AddRoute("/mcp/graphql/query").
		Wrap(allowsCORS, requireSage, wrapUserForMCP).
		Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlers.CompressHandler(http.HandlerFunc(graphql.Handler(uis.Settings.Api.URL, false))).ServeHTTP(w, r)
		})).
		Post().Get()

	// Waterfall pages
	app.AddRoute("/").Wrap(needsLogin, needsContext).Handler(uis.legacyWaterfallPage).Get().Head()
	app.AddRoute("/waterfall").Wrap(needsLogin, needsContext).Handler(uis.legacyWaterfallPage).Get()
	app.AddRoute("/waterfall/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyWaterfallPage).Get()

	// Task page (and related routes)
	app.AddRoute("/task/{task_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyTaskPage).Get()
	app.AddRoute("/task/{task_id}/{execution}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyTaskPage).Get()
	app.AddRoute("/json/task_log/{task_id}").Wrap(needsLogin, needsContext, viewLogs).Handler(uis.taskLog).Get()
	app.AddRoute("/json/task_log/{task_id}/{execution}").Wrap(needsLogin, needsContext, viewLogs).Handler(uis.taskLog).Get()
	app.AddRoute("/task_log_raw/{task_id}/{execution}").Wrap(needsLogin, needsContext, allowsCORS, viewLogs).Handler(uis.taskLogRaw).Get()

	// Proxy downloads for task uploaded files via S3
	app.AddRoute("/task_file_raw/{task_id}/{execution}/{file_name}").Wrap(needsLogin, needsContext, allowsCORS, viewLogs).Handler(uis.taskFileRaw).Get()
	// Test Logs
	app.AddRoute("/test_log/{task_id}/{task_execution}").Wrap(needsLogin, needsContext, allowsCORS, viewLogs).Handler(uis.testLog).Get()
	// TODO: We are keeping this route temporarily for backwards
	// compatibility. Please use
	// `/test_log/{task_id}/{task_execution}?test_name={test_name}`.
	app.AddRoute("/test_log/{task_id}/{task_execution}/{test_name}").Wrap(needsLogin, needsContext, allowsCORS, viewLogs).Handler(uis.testLog).Get()

	// Build page
	app.AddRoute("/build/{build_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyBuildPage).Get()

	// Version page
	app.AddRoute("/version/{version_id}").Wrap(needsLogin).Handler(uis.legacyVersionPage).Get()

	// Hosts
	app.AddRoute("/hosts").Handler(uis.legacyHostsPage).Get()
	app.AddRoute("/host/{host_id}").Handler(uis.legacyHostPage).Get()
	app.AddPrefixRoute("/host/{host_id}/ide/").Wrap(needsLogin, ownsHost, vsCodeRunning).Proxy(gimlet.ProxyOptions{
		FindTarget:        uis.getHostDNS,
		StripSourcePrefix: true,
		RemoteScheme:      "http",
		Transport:         rehttp.NewTransport(nil, rehttp.RetryAll(rehttp.RetryMaxRetries(5), rehttp.RetryTemporaryErr()), rehttp.ConstDelay(2*time.Second)),
		ErrorHandler:      uis.handleBackendError("IDE service is not available.\nEnsure the code-server service is running", http.StatusInternalServerError),
	}).AllMethods()
	// Prefix routes not ending in a '/' are not automatically redirected by gimlet's underlying library.
	// Add another route to match when there's no trailing slash and redirect
	app.AddRoute("/host/{host_id}/ide").Handler(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
	}).Get()

	// Distros
	app.AddRoute("/distros").Wrap(needsLogin, needsContext).Handler(uis.legacyDistrosPage).Get()

	// Task History
	app.AddRoute("/task_history/{task_name}").Wrap(needsLogin, needsContext).Handler(uis.legacyTaskHistoryPage).Get()
	app.AddRoute("/task_history/{project_id}/{task_name}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyTaskHistoryPage).Get()

	// Variant History
	app.AddRoute("/build_variant/{project_id}/{variant}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyVariantHistory).Get()

	// Task queues
	app.AddRoute("/task_queue/{distro}/{task_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyTaskQueue).Get()

	// Patch pages
	app.AddRoute("/patch/{patch_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyPatchPage).Get()
	app.AddRoute("/diff/{patch_id}/").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyDiffPage).Get()
	app.AddRoute("/filediff/{patch_id}/").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyFileDiffPage).Get()
	app.AddRoute("/rawdiff/{patch_id}/").Wrap(needsLogin, needsContext, allowsCORS, viewTasks).Handler(uis.rawDiffPage).Get()
	app.AddRoute("/patches").Wrap(needsLogin).Handler(uis.legacyPatchesPage).Get()
	app.AddRoute("/patches/project/{project_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyProjectPatchesPage).Get()
	app.AddRoute("/patches/user/{user_id}").Wrap(needsLogin).Handler(uis.legacyUserPatchesPage).Get()
	app.AddRoute("/patches/mine").Wrap(needsLogin).Handler(uis.legacyMyPatchesPage).Get()

	// Legacy Spawnhost routes - redirect to new UI
	app.AddRoute("/spawn").Wrap(needsLogin).Handler(uis.legacySpawnHostPage).Get().Put().Post()
	app.AddRoute("/spawn/hosts").Wrap(needsLogin).Handler(uis.legacySpawnHostPage).Get()
	app.AddRoute("/spawn/volumes").Wrap(needsLogin).Handler(uis.legacySpawnVolumePage).Get().Put()

	// User settings
	app.AddRoute("/settings").Wrap(needsLogin, needsContext).Handler(uis.legacyUserSettingsPage).Get()
	app.AddRoute("/notifications").Wrap(needsLogin, needsContext).Handler(uis.legacyNotificationsPage).Get()
	app.AddRoute("/preferences/cli").Wrap(needsLogin, needsContext).Handler(uis.reidrectUserPreferencesCLIPage).Get()

	// Task stats
	app.AddRoute("/task_timing").Wrap(needsLogin, needsContext).Handler(uis.legacyWaterfallPage).Get()
	app.AddRoute("/task_timing/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.legacyWaterfallPage).Get()

	// Project routes
	app.AddRoute("/projects/{project_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyProjectsPage).Get()

	// Deprecated Build Baron routes - redirect to new Spruce UI
	app.PrefixRoute("/plugin").Route("/buildbaron/jira_bf_search/{task_id}/{execution}").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/created_tickets/{task_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/custom_created_tickets/{task_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/note/{task_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/note/{task_id}").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Put()
	app.PrefixRoute("/plugin").Route("/buildbaron/file_ticket").Wrap(needsLogin, needsContext).Handler(uis.legacyBuildBaronPage).Post()

	// Add an OPTIONS method to every POST and GET request to handle preflight OPTIONS requests.
	// These requests must not check for credentials. They exist to validate whether a route exists, and to
	// allow requests from specific origins.
	for _, r := range app.Routes() {
		if r.HasMethod(http.MethodPost) || r.HasMethod(http.MethodGet) {
			app.AddRoute(r.GetRoute()).Wrap(allowsCORS).Handler(func(w http.ResponseWriter, _ *http.Request) { gimlet.WriteJSON(w, "") }).Options()
		}
	}

	return app
}
