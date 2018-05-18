package service

import (
	"fmt"
	htmlTemplate "html/template"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	ProjectCookieName string = "mci-project-cookie"
)

// UIServer provides a web interface for Evergreen.
type UIServer struct {
	render     gimlet.Renderer
	renderText gimlet.Renderer
	// Home is the root path on disk from which relative urls are constructed for loading
	// plugins or other assets.
	Home string

	// The root URL of the server, used in redirects for instance.
	RootURL string

	//authManager
	UserManager        auth.UserManager
	Settings           evergreen.Settings
	CookieStore        *sessions.CookieStore
	clientConfig       *evergreen.ClientConfig
	jiraHandler        thirdparty.JiraHandler
	buildBaronProjects map[string]evergreen.BuildBaronProject

	queue amboy.Queue

	plugin.PanelManager
}

// ViewData contains common data that is provided to all Evergreen pages
type ViewData struct {
	User        *user.DBUser
	ProjectData projectContext
	Project     model.Project
	Flashes     []interface{}
	Banner      string
	BannerTheme string
	Csrf        htmlTemplate.HTML
	JiraHost    string
}

func NewUIServer(settings *evergreen.Settings, queue amboy.Queue, home string, fo TemplateFunctionOptions) (*UIServer, error) {

	userManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, err
	}

	functions, err := MakeTemplateFuncs(fo, settings.SuperUsers)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create template function map")
	}

	ropts := gimlet.RendererOptions{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: !settings.Ui.CacheTemplates,
		Functions:    functions,
	}

	uis := &UIServer{
		Settings:           *settings,
		queue:              queue,
		Home:               home,
		UserManager:        userManager,
		clientConfig:       evergreen.GetEnvironment().ClientConfig(),
		CookieStore:        sessions.NewCookieStore([]byte(settings.Ui.Secret)),
		buildBaronProjects: bbGetConfig(settings),
		render:             gimlet.NewHTMLRenderer(ropts),
		renderText:         gimlet.NewTextRenderer(ropts),
		jiraHandler: thirdparty.NewJiraHandler(
			settings.Jira.GetHostURL(),
			settings.Jira.Username,
			settings.Jira.Password),
	}

	uis.PanelManager = &plugin.SimplePanelManager{}
	if err := uis.PanelManager.RegisterPlugins(plugin.UIPlugins); err != nil {
		return nil, errors.Wrap(err, "problem initializing plugins")
	}

	return uis, nil
}

// NewRouter sets up a request router for the UI, installing
// hard-coded routes as well as those belonging to plugins.
func (uis *UIServer) AttachRoutes(r *mux.Router) error {
	r = r.StrictSlash(true)

	// User login and logout
	r.HandleFunc("/login", uis.loginPage).Methods("GET")
	r.HandleFunc("/login", uis.login).Methods("POST")
	// User login with redirect to external site and redirect back
	r.HandleFunc("/login/redirect", uis.UserManager.GetLoginHandler(uis.RootURL)).Methods("GET")
	r.HandleFunc("/login/redirect/callback", uis.UserManager.GetLoginCallbackHandler()).Methods("GET")
	r.HandleFunc("/logout", uis.logout)

	requireLogin := func(next http.HandlerFunc) http.HandlerFunc {
		return requireUser(next, uis.RedirectToLogin)
	}

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
	r.HandleFunc("/tasks/{task_id}", requireLogin(uis.loadCtx(uis.taskModify))).Methods("PUT")
	r.HandleFunc("/json/task_log/{task_id}", uis.loadCtx(uis.taskLog))
	r.HandleFunc("/json/task_log/{task_id}/{execution}", uis.loadCtx(uis.taskLog))
	r.HandleFunc("/task_log_raw/{task_id}/{execution}", uis.loadCtx(uis.taskLogRaw))

	// Performance discovery pages
	r.HandleFunc("/perfdiscovery/", requireLogin(uis.loadCtx(uis.perfdiscoveryPage))).Methods("GET")
	r.HandleFunc("/perfdiscovery/{project_id}", requireLogin(uis.loadCtx(uis.perfdiscoveryPage))).Methods("GET")

	// Test Logs
	r.HandleFunc("/test_log/{task_id}/{task_execution}/{test_name}", uis.loadCtx(uis.testLog))
	r.HandleFunc("/test_log/{log_id}", uis.loadCtx(uis.testLog))

	// Build page
	r.HandleFunc("/build/{build_id}", uis.loadCtx(uis.buildPage)).Methods("GET")
	r.HandleFunc("/builds/{build_id}", requireLogin(uis.loadCtx(uis.modifyBuild))).Methods("PUT")
	r.HandleFunc("/json/build_history/{build_id}", uis.loadCtx(uis.buildHistory)).Methods("GET")

	// Version page
	r.HandleFunc("/version/{version_id}", uis.loadCtx(uis.versionPage)).Methods("GET")
	r.HandleFunc("/version/{version_id}", requireLogin(uis.loadCtx(uis.modifyVersion))).Methods("PUT")
	r.HandleFunc("/json/version_history/{version_id}", uis.loadCtx(uis.versionHistory))
	r.HandleFunc("/version/{project_id}/{revision}", uis.loadCtx(uis.versionFind)).Methods("GET")

	// Hosts
	r.HandleFunc("/hosts", requireLogin(uis.loadCtx(uis.hostsPage))).Methods("GET")
	r.HandleFunc("/hosts", requireLogin(uis.loadCtx(uis.modifyHosts))).Methods("PUT")
	r.HandleFunc("/host/{host_id}", requireLogin(uis.loadCtx(uis.hostPage))).Methods("GET")
	r.HandleFunc("/host/{host_id}", uis.requireSuperUser(uis.loadCtx(uis.modifyHost))).Methods("PUT")

	// Distros
	r.HandleFunc("/distros", requireLogin(uis.loadCtx(uis.distrosPage))).Methods("GET")
	r.HandleFunc("/distros", uis.requireSuperUser(uis.loadCtx(uis.addDistro))).Methods("PUT")
	r.HandleFunc("/distros/{distro_id}", requireLogin(uis.loadCtx(uis.getDistro))).Methods("GET")
	r.HandleFunc("/distros/{distro_id}", uis.requireSuperUser(uis.loadCtx(uis.addDistro))).Methods("PUT")
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
	r.HandleFunc("/task_queue/", requireLogin(uis.loadCtx(uis.allTaskQueues)))

	// Scheduler
	r.HandleFunc("/scheduler/distro/{distro_id}", uis.loadCtx(uis.getSchedulerPage))
	r.HandleFunc("/scheduler/distro/{distro_id}/logs", uis.loadCtx(uis.getSchedulerLogs))
	r.HandleFunc("/scheduler/stats", uis.loadCtx(uis.schedulerStatsPage))
	r.HandleFunc("/scheduler/distro/{distro_id}/stats", uis.loadCtx(uis.averageSchedulerStats))
	r.HandleFunc("/scheduler/stats/utilization", uis.loadCtx(uis.schedulerHostUtilization))

	// Patch pages
	r.HandleFunc("/patch/{patch_id}", requireLogin(uis.loadCtx(uis.patchPage))).Methods("GET")
	r.HandleFunc("/patch/{patch_id}", requireLogin(uis.loadCtx(uis.schedulePatch))).Methods("POST")
	r.HandleFunc("/diff/{patch_id}/", requireLogin(uis.loadCtx(uis.diffPage)))
	r.HandleFunc("/filediff/{patch_id}/", requireLogin(uis.loadCtx(uis.fileDiffPage)))
	r.HandleFunc("/rawdiff/{patch_id}/", requireLogin(uis.loadCtx(uis.rawDiffPage)))
	r.HandleFunc("/patches", requireLogin(uis.loadCtx(uis.patchTimeline)))
	r.HandleFunc("/patches/project/{project_id}", requireLogin(uis.loadCtx(uis.patchTimeline)))
	r.HandleFunc("/patches/user/{user_id}", requireLogin(uis.loadCtx(uis.userPatchesTimeline)))
	r.HandleFunc("/patches/mine", requireLogin(uis.loadCtx(uis.myPatchesTimeline)))

	// Spawnhost routes
	r.HandleFunc("/spawn", requireLogin(uis.loadCtx(uis.spawnPage))).Methods("GET")
	r.HandleFunc("/spawn", requireLogin(uis.loadCtx(uis.requestNewHost))).Methods("PUT")
	r.HandleFunc("/spawn", requireLogin(uis.loadCtx(uis.modifySpawnHost))).Methods("POST")
	r.HandleFunc("/spawn/hosts", requireLogin(uis.loadCtx(uis.getSpawnedHosts))).Methods("GET")
	r.HandleFunc("/spawn/distros", requireLogin(uis.loadCtx(uis.listSpawnableDistros))).Methods("GET")
	r.HandleFunc("/spawn/keys", requireLogin(uis.loadCtx(uis.getUserPublicKeys))).Methods("GET")

	// User settings
	r.HandleFunc("/settings", requireLogin(uis.loadCtx(uis.userSettingsPage))).Methods("GET")
	r.HandleFunc("/settings/newkey", requireLogin(uis.loadCtx(uis.newAPIKey))).Methods("POST")

	// Task stats
	r.HandleFunc("/task_timing", requireLogin(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/task_timing/{project_id}", requireLogin(uis.loadCtx(uis.taskTimingPage))).Methods("GET")
	r.HandleFunc("/json/task_timing/{project_id}/{build_variant}/{request}/{task_name}", requireLogin(uis.loadCtx(uis.taskTimingJSON))).Methods("GET")
	r.HandleFunc("/json/task_timing/{project_id}/{build_variant}/{request}", requireLogin(uis.loadCtx(uis.taskTimingJSON))).Methods("GET")

	// Project routes
	r.HandleFunc("/projects", requireLogin(uis.loadCtx(uis.projectsPage))).Methods("GET")
	r.HandleFunc("/project/{project_id}", uis.loadCtx(uis.requireAdmin(uis.projectPage))).Methods("GET")
	r.HandleFunc("/project/{project_id}", uis.loadCtx(uis.requireAdmin(uis.modifyProject))).Methods("POST")
	r.HandleFunc("/project/{project_id}", uis.loadCtx(uis.requireAdmin(uis.addProject))).Methods("PUT")
	r.HandleFunc("/project/{project_id}/repo_revision", uis.loadCtx(uis.requireAdmin(uis.setRevision))).Methods("PUT")

	// Admin routes
	r.HandleFunc("/admin", requireLogin(uis.loadCtx(uis.adminSettings))).Methods("GET")
	r.HandleFunc("/admin/events", requireLogin(uis.loadCtx(uis.adminEvents))).Methods("GET")

	// attaches /rest/v2 routes
	route.AttachHandler(r, uis.queue, uis.Settings.Ui.Url, evergreen.RestRoutePrefix, uis.Settings.SuperUsers, []byte(uis.Settings.Api.GithubWebhookSecret))

	// Static Path handlers
	r.PathPrefix("/clients").Handler(http.StripPrefix("/clients", http.FileServer(http.Dir(filepath.Join(uis.Home, evergreen.ClientDirectory)))))

	// Plugin routes
	rootPluginRouter := r.PathPrefix("/plugin").Subrouter()
	rootPluginRouter.HandleFunc("/buildbaron/jira_bf_search/{task_id}/{execution}", uis.bbJiraSearch)
	rootPluginRouter.HandleFunc("/buildbaron/created_tickets/{task_id}", uis.bbGetCreatedTickets)
	rootPluginRouter.HandleFunc("/buildbaron/note/{task_id}", bbGetNote).Methods("GET")
	rootPluginRouter.HandleFunc("/buildbaron/note/{task_id}", bbSaveNote).Methods("PUT")
	rootPluginRouter.HandleFunc("/buildbaron/file_ticket", uis.bbFileTicket).Methods("POST")
	rootPluginRouter.HandleFunc("/manifest/get/{project_id}/{revision}", uis.GetManifest)
	rootPluginRouter.HandleFunc("/dashboard/tasks/project/{project_id}/version/{version_id}", perfDashGetTasksForVersion)
	rootPluginRouter.HandleFunc("/json/version", perfGetVersion)
	rootPluginRouter.HandleFunc("/json/version/{version_id}/{name}", perfGetTasksForVersion)
	rootPluginRouter.HandleFunc("/json/version/latest/{project_id}/{name}", perfGetTasksForLatestVersion)
	rootPluginRouter.HandleFunc("/json/task/{task_id}/{name}/", perfGetTaskById)
	rootPluginRouter.HandleFunc("/json/task/{task_id}/{name}/tags", perfGetTags).Methods("POST", "DELETE")
	rootPluginRouter.HandleFunc("/json/task/{task_id}/{name}/tag", perfHandleTaskTag)
	rootPluginRouter.HandleFunc("/json/tags/", perfGetProjectTags)
	rootPluginRouter.HandleFunc("/json/tag/{project_id}/{tag}/{variant}/{task_name}/{name}", perfGetTaskJSONByTag)
	rootPluginRouter.HandleFunc("/json/commit/{project_id}/{revision}/{variant}/{task_name}/{name}", perfGetCommit)
	rootPluginRouter.HandleFunc("/json/history/{task_id}/{name}", perfGetTaskHistory)

	for _, pl := range plugin.UIPlugins {
		// get the settings
		pluginSettings := uis.Settings.Plugins[pl.Name()]
		err := pl.Configure(pluginSettings)
		if err != nil {
			return errors.Wrapf(err, "Failed to configure plugin %v", pl.Name())
		}

		// check if a plugin is an app level plugin first
		if appPlugin, ok := pl.(plugin.AppUIPlugin); ok {
			// register the app level pa}rt of the plugin
			appFunction := uis.GetPluginHandler(appPlugin.GetAppPluginInfo(), pl.Name())
			rootPluginRouter.HandleFunc(fmt.Sprintf("/%v/app", pl.Name()), uis.loadCtx(appFunction))
		}

		// check if there are any errors getting the panel config
		uiConf, err := pl.GetPanelConfig()
		if err != nil {
			return errors.Wrapf(err, "Error getting UI config for plugin %v: %v", pl.Name())
		}
		if uiConf == nil {
			grip.Debugf("No UI config needed for plugin %s, skipping", pl.Name())
			continue
		}

		plRouter := rootPluginRouter.PathPrefix(fmt.Sprintf("/%v/", pl.Name())).Subrouter()

		// set up a fileserver in plugin's static root, if one is provided
		pluginAssetsPath := filepath.Join(uis.Home, "public", "static", "plugins", pl.Name())

		grip.Infof("Registering assets path for plugin '%s' in %s", pl.Name(), pluginAssetsPath)
		plRouter.PathPrefix("/static/").Handler(
			http.StripPrefix(fmt.Sprintf("/plugin/%v/static/", pl.Name()),
				http.FileServer(http.Dir(pluginAssetsPath))),
		)
	}

	return nil
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
		"request": GetRequestID(r),
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

// GetCommonViewData returns a struct that can supplement the struct used to provide data to
// views. It contains data that is used for most/all Evergreen pages.
// The needsUser and needsProject params will cause an error to be logged if there is no
// user/project, but other data will still be returned
func (uis *UIServer) GetCommonViewData(w http.ResponseWriter, r *http.Request, needsUser, needsProject bool) ViewData {
	viewData := ViewData{}
	userCtx := GetUser(r)
	if needsUser && userCtx == nil {
		grip.Error("no user attached to request")
	}
	projectCtx, err := GetProjectContext(r)
	if err != nil {
		grip.Errorf(errors.Wrap(err, "error getting project context").Error())
		uis.ProjectNotFound(projectCtx, w, r)
		return ViewData{}
	}
	if needsProject {
		var project *model.Project
		project, err = projectCtx.GetProject()
		if err != nil || project == nil {
			grip.Errorf(errors.Wrap(err, "no project attached to request").Error())
			uis.ProjectNotFound(projectCtx, w, r)
			return ViewData{}
		}
		viewData.Project = *project
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		grip.Errorf(errors.Wrap(err, "unable to retrieve admin settings").Error())
	}
	viewData.Banner = settings.Banner
	viewData.BannerTheme = string(settings.BannerTheme)
	viewData.User = userCtx
	viewData.ProjectData = projectCtx
	viewData.Flashes = PopFlashes(uis.CookieStore, r, w)
	viewData.Csrf = csrf.TemplateField(r)
	viewData.JiraHost = uis.Settings.Jira.Host
	return viewData
}
