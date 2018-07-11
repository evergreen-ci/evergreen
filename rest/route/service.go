package route

import (
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(app *gimlet.APIApp, queue amboy.Queue, URL string, superUsers []string, githubSecret []byte) {
	sc := &data.DBConnector{}

	sc.SetURL(URL)
	sc.SetSuperUsers(superUsers)
	GetHandler(app, sc, queue, githubSecret)
}

// GetHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func GetHandler(app *gimlet.APIApp, sc data.Connector, queue amboy.Queue, githubSecret []byte) {
	routes := map[string]routeManagerFactory{
		"/alias/{name}":                      getAliasRouteManager,
		"/builds/{build_id}":                 getBuildByIdRouteManager,
		"/builds/{build_id}/abort":           getBuildAbortRouteManager,
		"/builds/{build_id}/restart":         getBuildRestartManager,
		"/builds/{build_id}/tasks":           getTasksByBuildRouteManager,
		"/cost/distro/{distro_id}":           getCostByDistroIdRouteManager,
		"/cost/project/{project_id}/tasks":   getCostTaskByProjectRouteManager,
		"/cost/version/{version_id}":         getCostByVersionIdRouteManager,
		"/distros":                           getDistroRouteManager,
		"/hooks/github":                      getGithubHooksRouteManager(queue, githubSecret),
		"/hosts":                             getHostRouteManager,
		"/hosts/{host_id}":                   getHostIDRouteManager,
		"/hosts/{host_id}/change_password":   getHostChangeRDPPasswordRouteManager,
		"/hosts/{host_id}/extend_expiration": getHostExtendExpirationRouteManager,
		"/hosts/{host_id}/terminate":         getHostTerminateRouteManager,
		"/keys":                                                getKeysRouteManager,
		"/keys/{key_name}":                                     getKeysDeleteRouteManager,
		"/patches/{patch_id}":                                  getPatchByIdManager,
		"/patches/{patch_id}/abort":                            getPatchAbortManager,
		"/patches/{patch_id}/restart":                          getPatchRestartManager,
		"/projects":                                            getProjectRouteManager,
		"/projects/{project_id}/patches":                       getPatchesByProjectManager,
		"/projects/{project_id}/recent_versions":               getRecentVersionsManager,
		"/projects/{project_id}/revisions/{commit_hash}/tasks": getTasksByProjectAndCommitRouteManager,
		"/status/cli_version":                                  getCLIVersionRouteManager,
		"/status/hosts/distros":                                getHostStatsByDistroManager,
		"/status/notifications":                                getNotificationsStatusRouteManager,
		"/status/recent_tasks":                                 getRecentTasksRouteManager,
		"/subscriptions":                                       getSubscriptionRouteManager,
		"/tasks/{task_id}":                                     getTaskRouteManager,
		"/tasks/{task_id}/abort":                               getTaskAbortManager,
		"/tasks/{task_id}/generate":                            getGenerateManager,
		"/tasks/{task_id}/metrics/process":                     getTaskProcessMetricsManager,
		"/tasks/{task_id}/metrics/system":                      getTaskSystemMetricsManager,
		"/tasks/{task_id}/restart":                             getTaskRestartRouteManager,
		"/tasks/{task_id}/tests":                               getTestRouteManager,
		"/user/settings":                                       getUserSettingsRouteManager,
		"/users/{user_id}/hosts":                               getHostsByUserManager,
		"/users/{user_id}/patches":                             getPatchesByUserManager,
		"/versions/{version_id}":                               getVersionIdRouteManager,
		"/versions/{version_id}/abort":                         getAbortVersionRouteManager,
		"/versions/{version_id}/builds":                        getBuildsForVersionRouteManager,
		"/versions/{version_id}/restart":                       getRestartVersionRouteManager,
	}

	for path, getManager := range routes {
		getManager(path, 2).Register(app, sc)
	}

	superUser := gimlet.NewRestrictAccessToUsers(sc.GetSuperUsers())
	checkUser := gimlet.NewRequireAuthHandler()

	app.AddRoute("/").Version(2).Get().RouteHandler(makePlaceHolderManger(sc))
	app.AddRoute("/admin").Version(2).Get().RouteHandler(makeLegacyAdminConfig(sc))
	app.AddRoute("/admin/banner").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchAdminBanner(sc))
	app.AddRoute("/admin/banner").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminBanner(sc))
	app.AddRoute("/admin/events").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminEvents(sc))
	app.AddRoute("/admin/revert").Version(2).Post().Wrap(superUser).RouteHandler(makeRevertRouteManager(sc))
	app.AddRoute("/admin/restart").Version(2).Post().Wrap(superUser).RouteHandler(makeRestartRoute(sc, queue))
	app.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(superUser).RouteHandler(makeClearTaskQueueHandler(sc))
	app.AddRoute("/admin/service_flags").Version(2).Post().Wrap(superUser).RouteHandler(makeSetServiceFlagsRouteManager(sc))
	app.AddRoute("/admin/settings").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminSettings(sc))
	app.AddRoute("/admin/settings").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminSettings(sc))
	app.AddRoute("/hosts/create/{task_id}").Version(2).Post().RouteHandler(makeHostCreateRouteManager(sc))
	app.AddRoute("/hosts/list/{task_id}").Version(2).Get().RouteHandler(makeHostListRouteManager(sc))
}
