package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/auth"
	"10gen.com/mci/plugin"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"net/http"
	"net/url"
	"strings"
)

const (
	ProjectKey        string = "projectKey"
	ProjectCookieName string = "mci-project-cookie"

	ProjectUnknown string = "Unknown Project"

	// Format string for when a project is not found
	ProjectNotFoundFormat string = "Project '%v' not found"
)

type UIServer struct {
	*render.Render

	// The root URL of the server, used in redirects for instance.
	RootURL string
	//authManager
	UserManager auth.UserManager

	MCISettings mci.MCISettings
	CookieStore *sessions.CookieStore

	plugin.PanelManager
}

func (uis *UIServer) InitPlugins() {
	uis.PanelManager = &plugin.SimplePanelManager{}
	uis.PanelManager.RegisterPlugins(plugin.Published)
}

func (uis *UIServer) NewRouter() (*mux.Router, error) {
	r := mux.NewRouter().StrictSlash(true)

	// User login and logout
	r.HandleFunc("/login", uis.loginPage).Methods("GET")
	r.HandleFunc("/login", uis.login).Methods("POST")
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

	// Buildmaster page
	r.HandleFunc("/buildmaster", uis.loadCtx(uis.buildmaster))
	r.HandleFunc("/buildmaster/{project_id}", uis.loadCtx(uis.buildmaster))
	r.HandleFunc("/buildmaster/{project_id}/{version_id}", uis.loadCtx(uis.buildmaster))

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

	// Event Logs
	r.HandleFunc("/event_log/{resource_type}/{resource_id:[\\w_\\-\\:\\.\\@]+}", uis.loadCtx(uis.fullEventLogs))

	// Task History
	r.HandleFunc("/task_history/{task_name}", uis.loadCtx(uis.taskHistoryPage))
	r.HandleFunc("/task_history/{project_id}/{task_name}", uis.loadCtx(uis.taskHistoryPage))
	r.HandleFunc("/task_history/{project_id}/{task_name}/pickaxe", uis.loadCtx(uis.taskHistoryPickaxe))
	r.HandleFunc("/task_history/{project_id}/{task_name}/test_names", uis.loadCtx(uis.taskHistoryTestNames))
	r.HandleFunc("/task_history_json/{task_id}/{window}", uis.loadCtx(uis.taskHistoryJson))

	// Task queues
	r.HandleFunc("/task_queue", uis.loadCtx(uis.allTaskQueues))

	// Patch pages
	r.HandleFunc("/patch/{patch_id}", uis.requireUser(uis.loadCtx(uis.patchPage))).Methods("GET")
	r.HandleFunc("/patch/{patch_id}", uis.requireUser(uis.loadCtx(uis.schedulePatch))).Methods("POST")
	r.HandleFunc("/diff/{patch_id}/", uis.requireUser(uis.loadCtx(uis.diffPage)))
	r.HandleFunc("/filediff/{patch_id}/", uis.requireUser(uis.loadCtx(uis.fileDiffPage)))
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

	// Task stats
	r.HandleFunc("/task_timing", uis.requireUser(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/task_timing/{project_id}", uis.requireUser(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/json/task_timing/{project_id}/{build_variant}/{task_name}", uis.requireUser(uis.loadCtx(uis.taskTimingJSON))).Methods("GET")

	// Plugin routes
	rootPluginRouter := r.PathPrefix("/plugin/").Subrouter()
	for _, pl := range plugin.Published {
		pluginSettings := uis.MCISettings.Plugins[pl.Name()]
		err := pl.Configure(pluginSettings)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure plugin %v: %v", pl.Name(), err)
		}
		uiConf, err := pl.GetPanelConfig()
		if err != nil {
			panic(fmt.Sprintf("Error getting UI config for plugin %v: %v", pl.Name(), err))
		}
		if uiConf == nil {
			mci.Logger.Logf(slogger.DEBUG, "No UI config needed for plugin %v, skipping", pl.Name())
			continue
		}
		// create a root path for the plugin based on its name
		plRouter := rootPluginRouter.PathPrefix(fmt.Sprintf("/%v/", pl.Name())).Subrouter()

		// set up a fileserver in plugin's static root, if one is provided
		if uiConf.StaticRoot != "" {
			mci.Logger.Logf(slogger.INFO, "Registering static path for plugin '%v' in %v", pl.Name(), uiConf.StaticRoot)
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
	mci.Logger.Logf(slogger.ERROR, fmt.Sprintf("%v %v %v", r.Method, r.URL, err.Error()))
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

func getRedirectPath(location string) string {
	parsedURL, err := url.Parse(location)
	if err != nil {
		return ""
	}
	return parsedURL.RawQuery[strings.Index(parsedURL.RawQuery, "=")+1:]
}
