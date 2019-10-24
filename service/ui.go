package service

import (
	htmlTemplate "html/template"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
	UserManager        gimlet.UserManager
	umconf             gimlet.UserMiddlewareConfiguration
	umIsLDAP           bool
	Settings           evergreen.Settings
	CookieStore        *sessions.CookieStore
	clientConfig       *evergreen.ClientConfig
	jiraHandler        thirdparty.JiraHandler
	buildBaronProjects map[string]evergreen.BuildBaronProject

	queue amboy.Queue
	env   evergreen.Environment

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
	Bugsnag     string
}

func NewUIServer(env evergreen.Environment, queue amboy.Queue, home string, fo TemplateFunctionOptions) (*UIServer, error) {
	settings := env.Settings()
	userManager, isLDAP, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	ropts := gimlet.RendererOptions{
		Directory:    filepath.Join(home, WebRootPath, Templates),
		DisableCache: !settings.Ui.CacheTemplates,
		Functions:    MakeTemplateFuncs(fo, settings.SuperUsers),
	}

	uis := &UIServer{
		Settings:           *settings,
		env:                env,
		queue:              queue,
		Home:               home,
		UserManager:        userManager,
		umIsLDAP:           isLDAP,
		clientConfig:       evergreen.GetEnvironment().ClientConfig(),
		CookieStore:        sessions.NewCookieStore([]byte(settings.Ui.Secret)),
		buildBaronProjects: bbGetConfig(settings),
		render:             gimlet.NewHTMLRenderer(ropts),
		renderText:         gimlet.NewTextRenderer(ropts),
		jiraHandler: thirdparty.NewJiraHandler(
			settings.Jira.GetHostURL(),
			settings.Jira.Username,
			settings.Jira.Password),
		umconf: gimlet.UserMiddlewareConfiguration{
			HeaderKeyName:  evergreen.APIKeyHeader,
			HeaderUserName: evergreen.APIUserHeader,
			CookieName:     evergreen.AuthTokenCookie,
			CookieTTL:      365 * 24 * time.Hour,
			CookiePath:     "/",
			CookieDomain:   settings.Ui.LoginDomain,
		},
	}

	if err := uis.umconf.Validate(); err != nil {
		return nil, errors.Wrap(err, "programmer error; invalid user middleware configuration")
	}

	plugins := plugin.GetPublished()
	uis.PanelManager = &plugin.SimplePanelManager{}

	if err := uis.PanelManager.RegisterPlugins(plugins); err != nil {
		return nil, errors.Wrap(err, "problem initializing plugins")
	}

	catcher := grip.NewBasicCatcher()
	for _, pl := range plugins {
		// get the settings
		catcher.Add(pl.Configure(uis.Settings.Plugins[pl.Name()]))
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
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

// GetCommonViewData returns a struct that can supplement the struct used to provide data to
// views. It contains data that is used for most/all Evergreen pages.
// The needsUser and needsProject params will cause an error to be logged if there is no
// user/project, but other data will still be returned
func (uis *UIServer) GetCommonViewData(w http.ResponseWriter, r *http.Request, needsUser, needsProject bool) ViewData {
	viewData := ViewData{}
	ctx := r.Context()
	userCtx := gimlet.GetUser(ctx)
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

	if u, ok := userCtx.(*user.DBUser); ok {
		viewData.User = u
	} else if userCtx != nil {
		grip.Criticalf("user [%s] is not of the correct type: %T", userCtx.Username(), userCtx)
	}

	viewData.Banner = settings.Banner
	viewData.BannerTheme = string(settings.BannerTheme)
	viewData.ProjectData = projectCtx
	viewData.Flashes = PopFlashes(uis.CookieStore, r, w)
	viewData.Csrf = csrf.TemplateField(r)
	viewData.JiraHost = uis.Settings.Jira.Host
	viewData.Bugsnag = settings.Bugsnag
	return viewData
}

// NewRouter sets up a request router for the UI, installing
// hard-coded routes as well as those belonging to plugins.
func (uis *UIServer) GetServiceApp() *gimlet.APIApp {
	needsLogin := gimlet.WrapperMiddleware(uis.requireLogin)
	needsContext := gimlet.WrapperMiddleware(uis.loadCtx)
	needsSuperUser := gimlet.WrapperMiddleware(uis.requireSuperUser)
	needsAdmin := gimlet.WrapperMiddleware(uis.requireAdmin)
	allowsCORS := gimlet.WrapperMiddleware(uis.setCORSHeaders)
	viewTasks := &route.RequiresProjectViewPermission{}
	editTasks := route.RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksBasic)
	viewLogs := route.RequiresProjectPermission(evergreen.PermissionLogs, evergreen.LogsView)
	submitPatches := route.RequiresProjectPermission(evergreen.PermissionPatches, evergreen.PatchSubmit)
	viewProjectSettings := route.RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView)
	editProjectSettings := route.RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsEdit)

	app := gimlet.NewApp()
	app.NoVersions = true

	// User login and logout
	app.AddRoute("/login").Handler(uis.loginPage).Get()
	app.AddRoute("/login").Handler(uis.login).Post()
	app.AddRoute("/login/key").Handler(uis.userGetKey).Post()
	app.AddRoute("/logout").Handler(uis.logout).Get()

	app.AddRoute("/robots.txt").Get().Handler(func(rw http.ResponseWriter, r *http.Request) {
		_, err := rw.Write([]byte(strings.Join([]string{
			"User-agent: *",
			"Disallow: /",
		}, "\n")))
		if err != nil {
			gimlet.WriteResponse(rw, gimlet.MakeTextErrorResponder(err))
		}
	})

	if h := uis.UserManager.GetLoginHandler(uis.RootURL); h != nil {
		app.AddRoute("/login/redirect").Handler(h).Get()
	}
	if h := uis.UserManager.GetLoginCallbackHandler(); h != nil {
		app.AddRoute("/login/redirect/callback").Handler(h).Get()
	}

	if uis.Settings.Ui.CsrfKey != "" {
		app.AddMiddleware(gimlet.WrapperHandlerMiddleware(
			csrf.Protect([]byte(uis.Settings.Ui.CsrfKey), csrf.ErrorHandler(http.HandlerFunc(ForbiddenHandler))),
		))
	}

	// Waterfall pages
	app.AddRoute("/").Wrap(needsContext).Handler(uis.waterfallPage).Get()
	app.AddRoute("/waterfall").Wrap(needsContext).Handler(uis.waterfallPage).Get()
	app.AddRoute("/waterfall/{project_id}").Wrap(needsContext, viewTasks).Handler(uis.waterfallPage).Get()

	// Timeline page

	app.AddRoute("/timeline/{project_id}").Wrap(needsContext, viewTasks).Handler(uis.timeline).Get()
	app.AddRoute("/timeline").Wrap(needsContext).Handler(uis.timeline).Get()
	app.AddRoute("/json/timeline/{project_id}").Wrap(needsContext, allowsCORS, needsLogin, viewTasks).Handler(uis.timelineJson).Get()
	app.AddRoute("/json/patches/project/{project_id}").Wrap(needsContext, allowsCORS, needsLogin, viewTasks).Handler(uis.patchTimelineJson).Get()
	app.AddRoute("/json/patches/user/{user_id}").Wrap(needsContext, allowsCORS, needsLogin, viewTasks).Handler(uis.patchTimelineJson).Get()

	// Grid page
	app.AddRoute("/grid").Wrap(needsContext).Handler(uis.grid).Get()
	app.AddRoute("/grid/{project_id}").Wrap(needsContext, viewTasks).Handler(uis.grid).Get()
	app.AddRoute("/grid/{project_id}/{version_id}").Wrap(needsContext, viewTasks).Handler(uis.grid).Get()
	app.AddRoute("/grid/{project_id}/{version_id}/{depth}").Wrap(needsContext, viewTasks).Handler(uis.grid).Get()

	// Task page (and related routes)
	app.AddRoute("/task/{task_id}").Wrap(needsContext, viewTasks).Handler(uis.taskPage).Get()
	app.AddRoute("/task/{task_id}/{execution}").Wrap(needsContext, viewTasks).Handler(uis.taskPage).Get()
	app.AddRoute("/tasks/{task_id}").Wrap(needsLogin, needsContext, editTasks).Handler(uis.taskModify).Put()
	app.AddRoute("/json/task_log/{task_id}").Wrap(needsContext, viewLogs).Handler(uis.taskLog).Get()
	app.AddRoute("/json/task_log/{task_id}/{execution}").Wrap(needsContext, viewLogs).Handler(uis.taskLog).Get()
	app.AddRoute("/task_log_raw/{task_id}/{execution}").Wrap(needsContext, allowsCORS, viewLogs).Handler(uis.taskLogRaw).Get()

	// Performance Discovery pages
	app.AddRoute("/perfdiscovery/").Wrap(needsLogin, needsContext).Handler(uis.perfdiscoveryPage).Get()
	app.AddRoute("/perfdiscovery/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.perfdiscoveryPage).Get()

	// Signal Processing page (UI-routing enabled for this route)
	app.AddRoute("/perf-bb/{_}/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.signalProcessingPage).Get()

	// Test Logs
	app.AddRoute("/test_log/{task_id}/{task_execution}/{test_name}").Wrap(needsContext, allowsCORS, viewLogs).Handler(uis.testLog).Get()
	app.AddRoute("/test_log/{log_id}").Wrap(needsContext, allowsCORS).Handler(uis.testLog).Get()

	// Build page
	app.AddRoute("/build/{build_id}").Wrap(needsContext, viewTasks).Handler(uis.buildPage).Get()
	app.AddRoute("/builds/{build_id}").Wrap(needsLogin, needsContext, editTasks).Handler(uis.modifyBuild).Put()
	app.AddRoute("/json/build_history/{build_id}").Wrap(needsContext, viewTasks).Handler(uis.buildHistory).Get()

	// Version page
	app.AddRoute("/version/{version_id}").Wrap(needsContext, viewTasks).Handler(uis.versionPage).Get()
	app.AddRoute("/version/{version_id}").Wrap(needsLogin, needsContext, editTasks).Handler(uis.modifyVersion).Put()
	app.AddRoute("/json/version_history/{version_id}").Wrap(needsContext, viewTasks).Handler(uis.versionHistory).Get()
	app.AddRoute("/version/{project_id}/{revision}").Wrap(needsContext, viewTasks).Handler(uis.versionFind).Get()

	// Hosts
	app.AddRoute("/hosts").Wrap(needsLogin, needsContext).Handler(uis.hostsPage).Get()
	app.AddRoute("/hosts").Wrap(needsLogin, needsContext).Handler(uis.modifyHosts).Put()
	app.AddRoute("/host/{host_id}").Wrap(needsLogin, needsContext).Handler(uis.hostPage).Get()
	app.AddRoute("/host/{host_id}").Wrap(needsSuperUser, needsContext).Handler(uis.modifyHost).Put()

	// Distros
	app.AddRoute("/distros").Wrap(needsLogin, needsContext).Handler(uis.distrosPage).Get()
	app.AddRoute("/distros").Wrap(needsSuperUser, needsContext).Handler(uis.addDistro).Put()
	app.AddRoute("/distros/{distro_id}").Wrap(needsLogin, needsContext).Handler(uis.getDistro).Get()
	app.AddRoute("/distros/{distro_id}").Wrap(needsSuperUser, needsContext).Handler(uis.addDistro).Put()
	app.AddRoute("/distros/{distro_id}").Wrap(needsSuperUser, needsContext).Handler(uis.modifyDistro).Post()
	app.AddRoute("/distros/{distro_id}").Wrap(needsSuperUser, needsContext).Handler(uis.removeDistro).Delete()

	// Event Logs
	app.AddRoute("/event_log/{resource_type}/{resource_id:[\\w_\\-\\:\\.\\@]+}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.fullEventLogs).Get()

	// Task History
	app.AddRoute("/task_history/{task_name}").Wrap(needsContext).Handler(uis.taskHistoryPage).Get()
	app.AddRoute("/task_history/{project_id}/{task_name}").Wrap(needsContext, viewTasks).Handler(uis.taskHistoryPage).Get()
	app.AddRoute("/task_history/{project_id}/{task_name}/pickaxe").Wrap(needsContext, viewTasks).Handler(uis.taskHistoryPickaxe).Get()
	app.AddRoute("/task_history/{project_id}/{task_name}/test_names").Wrap(needsContext, viewTasks).Handler(uis.taskHistoryTestNames).Get()

	// History Drawer Endpoints
	app.AddRoute("/history/tasks/2/{version_id}/{window}/{variant}/{display_name}").Wrap(needsContext, viewTasks).Handler(uis.taskHistoryDrawer).Get()
	app.AddRoute("/history/versions/{version_id}/{window}").Wrap(needsContext, viewTasks).Handler(uis.versionHistoryDrawer).Get()

	// Variant History
	app.AddRoute("/build_variant/{project_id}/{variant}").Wrap(needsContext, viewTasks).Handler(uis.variantHistory).Get()

	// Task queues
	app.AddRoute("/task_queue/").Wrap(needsLogin, needsContext).Handler(uis.allTaskQueues).Get() // TODO: ¯\_(ツ)_/¯

	// Scheduler
	app.AddRoute("/scheduler/distro/{distro_id}").Wrap(needsContext).Handler(uis.getSchedulerPage).Get()
	app.AddRoute("/scheduler/distro/{distro_id}/logs").Wrap(needsContext).Handler(uis.getSchedulerLogs).Get()
	app.AddRoute("/scheduler/stats").Wrap(needsContext).Handler(uis.schedulerStatsPage).Get()
	app.AddRoute("/scheduler/distro/{distro_id}/stats").Wrap(needsContext).Handler(uis.averageSchedulerStats).Get()
	app.AddRoute("/scheduler/stats/utilization").Wrap(needsContext).Handler(uis.schedulerHostUtilization).Get()

	// Patch pages
	app.AddRoute("/patch/{patch_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.patchPage).Get()
	app.AddRoute("/patch/{patch_id}").Wrap(needsLogin, needsContext, submitPatches).Handler(uis.schedulePatch).Post()
	app.AddRoute("/diff/{patch_id}/").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.diffPage).Get()
	app.AddRoute("/filediff/{patch_id}/").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.fileDiffPage).Get()
	app.AddRoute("/rawdiff/{patch_id}/").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.rawDiffPage).Get()
	app.AddRoute("/patches").Wrap(needsLogin, needsContext).Handler(uis.patchTimeline).Get()
	app.AddRoute("/patches/project/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.patchTimeline).Get()
	app.AddRoute("/patches/user/{user_id}").Wrap(needsLogin, needsContext).Handler(uis.userPatchesTimeline).Get()
	app.AddRoute("/patches/mine").Wrap(needsLogin, needsContext).Handler(uis.myPatchesTimeline).Get()

	// Spawnhost routes
	app.AddRoute("/spawn").Wrap(needsLogin, needsContext).Handler(uis.spawnPage).Get()
	app.AddRoute("/spawn").Wrap(needsLogin, needsContext).Handler(uis.requestNewHost).Put()
	app.AddRoute("/spawn").Wrap(needsLogin, needsContext).Handler(uis.modifySpawnHost).Post()
	app.AddRoute("/spawn/hosts").Wrap(needsLogin, needsContext).Handler(uis.getSpawnedHosts).Get()
	app.AddRoute("/spawn/distros").Wrap(needsLogin, needsContext).Handler(uis.listSpawnableDistros).Get()
	app.AddRoute("/spawn/keys").Wrap(needsLogin, needsContext).Handler(uis.getUserPublicKeys).Get()
	app.AddRoute("/spawn/types").Wrap(needsLogin, needsContext).Handler(uis.getAllowedInstanceTypes).Get()

	// User settings
	app.AddRoute("/settings").Wrap(needsLogin, needsContext).Handler(uis.userSettingsPage).Get()
	app.AddRoute("/settings/newkey").Wrap(needsLogin, needsContext).Handler(uis.newAPIKey).Post()
	app.AddRoute("/settings/cleartoken").Wrap(needsLogin).Handler(uis.clearUserToken).Post()
	app.AddRoute("/notifications").Wrap(needsLogin, needsContext).Handler(uis.notificationsPage).Get()

	// Task stats
	app.AddRoute("/task_timing").Wrap(needsLogin, needsContext).Handler(uis.taskTimingPage).Get()
	app.AddRoute("/task_timing/{project_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.taskTimingPage).Get()
	app.AddRoute("/json/task_timing/{project_id}/{build_variant}/{request}/{task_name}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.taskTimingJSON).Get()
	app.AddRoute("/json/task_timing/{project_id}/{build_variant}/{request}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.taskTimingJSON).Get()

	// Project routes
	app.AddRoute("/projects").Wrap(needsLogin, needsContext).Handler(uis.projectsPage).Get()
	app.AddRoute("/project/{project_id}").Wrap(needsContext, needsAdmin, viewProjectSettings).Handler(uis.projectPage).Get()
	app.AddRoute("/project/{project_id}/events").Wrap(needsContext, needsAdmin, viewProjectSettings).Handler(uis.projectEvents).Get()
	app.AddRoute("/project/{project_id}").Wrap(needsContext, needsAdmin, editProjectSettings).Handler(uis.modifyProject).Post()
	app.AddRoute("/project/{project_id}").Wrap(needsContext, needsAdmin, editProjectSettings).Handler(uis.addProject).Put()
	app.AddRoute("/project/{project_id}/repo_revision").Wrap(needsContext, editProjectSettings).Handler(uis.setRevision).Put()

	// Admin routes
	app.AddRoute("/admin").Wrap(needsLogin, needsContext).Handler(uis.adminSettings).Get()
	app.AddRoute("/admin/cleartokens").Wrap(needsSuperUser).Handler(uis.clearAllUserTokens).Post()
	app.AddRoute("/admin/events").Wrap(needsLogin, needsContext).Handler(uis.adminEvents).Get()

	// Plugin routes
	app.PrefixRoute("/plugin").Route("/buildbaron/jira_bf_search/{task_id}/{execution}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.bbJiraSearch).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/created_tickets/{task_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.bbGetCreatedTickets).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/note/{task_id}").Wrap(needsLogin, needsContext, viewTasks).Handler(bbGetNote).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/note/{task_id}").Wrap(needsLogin, needsContext, editTasks).Handler(bbSaveNote).Put()
	app.PrefixRoute("/plugin").Route("/buildbaron/file_ticket").Wrap(needsLogin, needsContext).Handler(uis.bbFileTicket).Post()
	app.PrefixRoute("/plugin").Route("/buildbaron/feedback/{task_id}/{execution}").Wrap(needsLogin, needsContext, viewTasks).Handler(uis.bbGetFeedback).Get()
	app.PrefixRoute("/plugin").Route("/buildbaron/feedback/{task_id}/{execution}/{feedback_type}").Wrap(needsLogin, needsContext, editTasks).Handler(uis.bbRemoveFeedback).Delete()
	app.PrefixRoute("/plugin").Route("/buildbaron/feedback").Wrap(needsLogin, needsContext).Handler(uis.bbSendFeedback).Post()
	app.PrefixRoute("/plugin").Route("/manifest/get/{project_id}/{revision}").Wrap(needsLogin, viewTasks).Handler(uis.GetManifest).Get()
	app.PrefixRoute("/plugin").Route("/dashboard/tasks/project/{project_id}/version/{version_id}").Wrap(needsLogin, viewTasks).Handler(perfDashGetTasksForVersion).Get()
	app.PrefixRoute("/plugin").Route("/json/version").Handler(perfGetVersion).Get()
	app.PrefixRoute("/plugin").Route("/json/version/{version_id}/{name}").Wrap(needsLogin, viewTasks).Handler(perfGetTasksForVersion).Get()
	app.PrefixRoute("/plugin").Route("/json/version/latest/{project_id}/{name}").Wrap(needsLogin, viewTasks).Handler(perfGetTasksForLatestVersion).Get()
	app.PrefixRoute("/plugin").Route("/json/task/{task_id}/{name}/").Wrap(needsLogin, viewTasks).Handler(perfGetTaskById).Get()
	app.PrefixRoute("/plugin").Route("/json/task/{task_id}/{name}/tags").Wrap(needsLogin, viewTasks).Handler(perfGetTags).Get()
	app.PrefixRoute("/plugin").Route("/json/task/{task_id}/{name}/tag").Wrap(needsLogin, editTasks).Handler(perfHandleTaskTag).Post().Delete()
	app.PrefixRoute("/plugin").Route("/json/tags/").Handler(perfGetProjectTags).Get()
	app.PrefixRoute("/plugin").Route("/json/tag/{project_id}/{tag}/{variant}/{task_name}/{name}").Wrap(needsLogin, viewTasks).Handler(perfGetTaskJSONByTag).Get()
	app.PrefixRoute("/plugin").Route("/json/commit/{project_id}/{revision}/{variant}/{task_name}/{name}").Wrap(needsLogin, viewTasks).Handler(perfGetCommit).Get()
	app.PrefixRoute("/plugin").Route("/json/history/{task_id}/{name}").Wrap(needsLogin, viewTasks).Handler(perfGetTaskHistory).Get()

	return app
}
