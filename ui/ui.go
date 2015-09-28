package ui

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
)

const (
	ProjectKey        string = "projectKey"
	ProjectCookieName string = "mci-project-cookie"

	ProjectUnknown string = "Unknown Project"

	// Format string for when a project is not found
	ProjectNotFoundFormat string = "Project '%v' not found"
)

// UIServer provides a web interface for Evergreen.
type UIServer struct {
	*render.Render

	// The root URL of the server, used in redirects for instance.
	RootURL string
	//authManager
	UserManager auth.UserManager
	Settings    evergreen.Settings
	CookieStore *sessions.CookieStore

	plugin.PanelManager
}

// InitPlugins registers all installed plugins with the UI Server.
func (uis *UIServer) InitPlugins() error {
	uis.PanelManager = &plugin.SimplePanelManager{}
	return uis.PanelManager.RegisterPlugins(plugin.UIPlugins)
}

// NewRouter sets up a request router for the UI, installing
// hard-coded routes as well as those belonging to plugins.
func (uis *UIServer) NewRouter() (*mux.Router, error) {
	r := mux.NewRouter().StrictSlash(true)

	// User login and logout
	r.HandleFunc("/login", uis.loginPage).Methods("GET")
	r.HandleFunc("/login", uis.login).Methods("POST")

	// User login with redirect to external site and redirect back
	if uis.UserManager.GetLoginHandler != nil {
		r.HandleFunc("/login/redirect", uis.UserManager.GetLoginHandler(uis.RootURL)).Methods("GET")
	}
	if uis.UserManager.GetLoginCallbackHandler != nil {
		r.HandleFunc("/login/redirect/callback", uis.UserManager.GetLoginCallbackHandler()).Methods("GET")
	}
	r.HandleFunc("/logout", uis.logout)

	// Waterfall pages
	r.HandleFunc("/", uis.loadCtx(uis.waterfallPage))
	r.HandleFunc("/waterfall", uis.loadCtx(uis.waterfallPage))
	r.HandleFunc("/waterfall/{project_id}", uis.loadCtx(uis.waterfallPage))

	// Timeline page
	r.HandleFunc("/timeline/{project_id}", uis.loadCtx(uis.timeline))
	r.HandleFunc("/timeline", uis.loadCtx(uis.timeline))
	r.HandleFunc("/json/timeline/{project_id}", uis.loadCtx(uis.timelineJson))
	r.HandleFunc("/json/patches/project/{project_id}", uis.loadCtx(uis.patchTimelineJson))
	r.HandleFunc("/json/patches/user/{user_id}", uis.loadCtx(uis.patchTimelineJson))

	// Grid page
	r.HandleFunc("/grid", uis.loadCtx(uis.grid))
	r.HandleFunc("/grid/{project_id}", uis.loadCtx(uis.grid))
	r.HandleFunc("/grid/{project_id}/{version_id}", uis.loadCtx(uis.grid))
	r.HandleFunc("/grid/{project_id}/{version_id}/{depth}", uis.loadCtx(uis.grid))

	// Task page (and related routes)
	r.HandleFunc("/task/{task_id}", uis.loadCtx(uis.taskPage)).Methods("GET")
	r.HandleFunc("/task/{task_id}/{execution}", uis.loadCtx(uis.taskPage)).Methods("GET")
	r.HandleFunc("/tasks/{task_id}", uis.requireUser(uis.loadCtx(uis.taskModify))).Methods("PUT")
	r.HandleFunc("/json/task_log/{task_id}", uis.loadCtx(uis.taskLog))
	r.HandleFunc("/json/task_log/{task_id}/{execution}", uis.loadCtx(uis.taskLog))
	r.HandleFunc("/task_log_raw/{task_id}/{execution}", uis.loadCtx(uis.taskLogRaw))
	r.HandleFunc("/task/dependencies/{task_id}", uis.loadCtx(uis.taskDependencies))
	r.HandleFunc("/task/dependencies/{task_id}/{execution}", uis.loadCtx(uis.taskDependencies))

	// Test Logs
	r.HandleFunc("/test_log/{task_id}/{task_execution}/{test_name}", uis.loadCtx(uis.testLog))
	r.HandleFunc("/test_log/{log_id}", uis.loadCtx(uis.testLog))

	// Build page
	r.HandleFunc("/build/{build_id}", uis.loadCtx(uis.buildPage)).Methods("GET")
	r.HandleFunc("/builds/{build_id}", uis.requireUser(uis.loadCtx(uis.modifyBuild))).Methods("PUT")
	r.HandleFunc("/json/last_green/{project_id}", uis.loadCtx(uis.lastGreenHandler)).Methods("GET")
	r.HandleFunc("/json/build_history/{build_id}", uis.loadCtx(uis.buildHistory)).Methods("GET")

	// Version page
	r.HandleFunc("/version/{version_id}", uis.loadCtx(uis.versionPage)).Methods("GET")
	r.HandleFunc("/version/{version_id}", uis.requireUser(uis.loadCtx(uis.modifyVersion))).Methods("PUT")
	r.HandleFunc("/json/version_history/{version_id}", uis.loadCtx(uis.versionHistory))

	// Hosts
	r.HandleFunc("/hosts", uis.loadCtx(uis.hostsPage)).Methods("GET")
	r.HandleFunc("/hosts", uis.requireUser(uis.loadCtx(uis.modifyHosts))).Methods("PUT")
	r.HandleFunc("/host/{host_id}", uis.loadCtx(uis.hostPage)).Methods("GET")
	r.HandleFunc("/host/{host_id}", uis.requireUser(uis.loadCtx(uis.modifyHost))).Methods("PUT")

	// Distros
	r.HandleFunc("/distros", uis.requireSuperUser(uis.loadCtx(uis.distrosPage))).Methods("GET")

	r.HandleFunc("/distros", uis.requireSuperUser(uis.loadCtx(uis.addDistro))).Methods("PUT")
	r.HandleFunc("/distros/{distro_id}", uis.requireSuperUser(uis.loadCtx(uis.getDistro))).Methods("GET")
	r.HandleFunc("/distros/{distro_id}", uis.requireSuperUser(uis.loadCtx(uis.modifyDistro))).Methods("POST")
	r.HandleFunc("/distros/{distro_id}", uis.requireSuperUser(uis.loadCtx(uis.removeDistro))).Methods("DELETE")

	// Event Logs
	r.HandleFunc("/event_log/{resource_type}/{resource_id:[\\w_\\-\\:\\.\\@]+}", uis.loadCtx(uis.fullEventLogs))

	// Task History
	r.HandleFunc("/task_history/{task_name}", uis.loadCtx(uis.taskHistoryPage))
	r.HandleFunc("/task_history/{project_id}/{task_name}", uis.loadCtx(uis.taskHistoryPage))
	r.HandleFunc("/task_history/{project_id}/{task_name}/pickaxe", uis.loadCtx(uis.taskHistoryPickaxe))
	r.HandleFunc("/task_history/{project_id}/{task_name}/test_names", uis.loadCtx(uis.taskHistoryTestNames))

	// History Drawer Endpoints
	r.HandleFunc("/history/tasks/{task_id}/{window}", uis.loadCtx(uis.taskHistoryDrawer))
	r.HandleFunc("/history/versions/{version_id}/{window}", uis.loadCtx(uis.versionHistoryDrawer))

	// Variant History
	r.HandleFunc("/build_variant/{project_id}/{variant}", uis.loadCtx(uis.variantHistory))

	// Task queues
	r.HandleFunc("/task_queue", uis.loadCtx(uis.allTaskQueues))

	// Patch pages
	r.HandleFunc("/patch/{patch_id}", uis.requireUser(uis.loadCtx(uis.patchPage))).Methods("GET")
	r.HandleFunc("/patch/{patch_id}", uis.requireUser(uis.loadCtx(uis.schedulePatch))).Methods("POST")
	r.HandleFunc("/diff/{patch_id}/", uis.requireUser(uis.loadCtx(uis.diffPage)))
	r.HandleFunc("/filediff/{patch_id}/", uis.requireUser(uis.loadCtx(uis.fileDiffPage)))
	r.HandleFunc("/rawdiff/{patch_id}/", uis.requireUser(uis.loadCtx(uis.rawDiffPage)))
	r.HandleFunc("/patches", uis.requireUser(uis.loadCtx(uis.patchTimeline)))
	r.HandleFunc("/patches/project/{project_id}", uis.requireUser(uis.loadCtx(uis.patchTimeline)))
	r.HandleFunc("/patches/user/{user_id}", uis.requireUser(uis.loadCtx(uis.userPatchesTimeline)))
	r.HandleFunc("/patches/mine", uis.requireUser(uis.loadCtx(uis.myPatchesTimeline)))

	// Spawnhost routes
	r.HandleFunc("/spawn", uis.requireUser(uis.loadCtx(uis.spawnPage))).Methods("GET")
	r.HandleFunc("/spawn", uis.requireUser(uis.loadCtx(uis.requestNewHost))).Methods("PUT")
	r.HandleFunc("/spawn", uis.requireUser(uis.loadCtx(uis.modifySpawnHost))).Methods("POST")
	r.HandleFunc("/spawn/hosts", uis.requireUser(uis.loadCtx(uis.getSpawnedHosts))).Methods("GET")
	r.HandleFunc("/spawn/distros", uis.requireUser(uis.loadCtx(uis.listSpawnableDistros))).Methods("GET")
	r.HandleFunc("/spawn/keys", uis.requireUser(uis.loadCtx(uis.getUserPublicKeys))).Methods("GET")

	// User settings
	r.HandleFunc("/settings", uis.requireUser(uis.loadCtx(uis.userSettingsPage))).Methods("GET")
	r.HandleFunc("/settings", uis.requireUser(uis.loadCtx(uis.userSettingsModify))).Methods("PUT")
	r.HandleFunc("/settings/newkey", uis.requireUser(uis.loadCtx(uis.newAPIKey))).Methods("POST")

	// Task stats
	r.HandleFunc("/task_timing", uis.requireUser(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/task_timing/{project_id}", uis.requireUser(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/json/task_timing/{project_id}/{build_variant}/{task_name}", uis.requireUser(uis.loadCtx(uis.taskTimingJSON))).Methods("GET")

	// Project routes
	r.HandleFunc("/projects", uis.requireUser(uis.loadCtx(uis.projectsPage))).Methods("GET")
	r.HandleFunc("/project/{project_id}", uis.requireSuperUser(uis.loadCtx(uis.projectPage))).Methods("GET")
	r.HandleFunc("/project/{project_id}", uis.requireSuperUser(uis.loadCtx(uis.modifyProject))).Methods("POST")
	r.HandleFunc("/project/{project_id}", uis.requireSuperUser(uis.loadCtx(uis.addProject))).Methods("PUT")
	r.HandleFunc("/project/{project_id}/repo_revision", uis.requireSuperUser(uis.loadCtx(uis.setRevision))).Methods("PUT")

	restRouter := r.PathPrefix("/rest/v1/").Subrouter().StrictSlash(true)
	restRoutes := rest.GetRestRoutes(uis)

	for _, restRoute := range restRoutes {
		restRouter.HandleFunc(restRoute.Path, uis.loadCtx(restRoute.Handler)).Name(restRoute.Name).Methods(restRoute.Method)
	}

	// Plugin routes
	rootPluginRouter := r.PathPrefix("/plugin/").Subrouter()
	for _, pl := range plugin.UIPlugins {
		pluginSettings := uis.Settings.Plugins[pl.Name()]
		err := pl.Configure(pluginSettings)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure plugin %v: %v", pl.Name(), err)
		}
		uiConf, err := pl.GetPanelConfig()
		if err != nil {
			panic(fmt.Sprintf("Error getting UI config for plugin %v: %v", pl.Name(), err))
		}
		if uiConf == nil {
			evergreen.Logger.Logf(slogger.DEBUG, "No UI config needed for plugin %v, skipping", pl.Name())
			continue
		}
		// create a root path for the plugin based on its name
		plRouter := rootPluginRouter.PathPrefix(fmt.Sprintf("/%v/", pl.Name())).Subrouter()

		// set up a fileserver in plugin's static root, if one is provided
		if uiConf.StaticRoot != "" {
			evergreen.Logger.Logf(slogger.INFO, "Registering static path for plugin '%v' in %v", pl.Name(), uiConf.StaticRoot)
			plRouter.PathPrefix("/static/").Handler(
				http.StripPrefix(fmt.Sprintf("/plugin/%v/static/", pl.Name()),
					http.FileServer(http.Dir(uiConf.StaticRoot))),
			)
		}
		pluginUIhandler := pl.GetUIHandler()
		util.MountHandler(rootPluginRouter, fmt.Sprintf("/%v/", pl.Name()), pluginUIhandler)
	}

	return r, nil
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (uis *UIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	evergreen.Logger.Logf(slogger.ERROR, fmt.Sprintf("%v %v %v", r.Method, r.URL, err.Error()))
	// if JSON is the preferred content type for the request, reply with a json message
	if strings.HasPrefix(r.Header.Get("accept"), "application/json") {
		uis.WriteJSON(w, code, struct {
			Error string `json:"error"`
		}{err.Error()})
	} else {
		// Not a JSON request, so write plaintext.
		http.Error(w, err.Error(), code)
	}
}
