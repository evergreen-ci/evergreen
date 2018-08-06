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

	routes := map[string]routeManagerFactory{
		"/distros":                           getDistroRouteManager,
		"/hooks/github":                      getGithubHooksRouteManager(queue, githubSecret),
		"/hosts/{host_id}/change_password":   getHostChangeRDPPasswordRouteManager,
		"/hosts/{host_id}/extend_expiration": getHostExtendExpirationRouteManager,
		"/hosts/{host_id}/terminate":         getHostTerminateRouteManager,
		"/keys":                                  getKeysRouteManager,
		"/keys/{key_name}":                       getKeysDeleteRouteManager,
		"/patches/{patch_id}/abort":              getPatchAbortManager,
		"/patches/{patch_id}/restart":            getPatchRestartManager,
		"/projects":                              getProjectRouteManager,
		"/projects/{project_id}/patches":         getPatchesByProjectManager,
		"/projects/{project_id}/recent_versions": getRecentVersionsManager,
		"/subscriptions":                         getSubscriptionRouteManager,
		"/tasks/{task_id}/tests":                 getTestRouteManager,
		"/user/settings":                         getUserSettingsRouteManager,
		"/versions/{version_id}/abort":           getAbortVersionRouteManager,
		"/versions/{version_id}/restart":         getRestartVersionRouteManager,
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
	app.AddRoute("/admin/restart").Version(2).Post().Wrap(superUser).RouteHandler(makeRestartRoute(sc, queue))
	app.AddRoute("/admin/revert").Version(2).Post().Wrap(superUser).RouteHandler(makeRevertRouteManager(sc))
	app.AddRoute("/admin/service_flags").Version(2).Post().Wrap(superUser).RouteHandler(makeSetServiceFlagsRouteManager(sc))
	app.AddRoute("/admin/settings").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminSettings(sc))
	app.AddRoute("/admin/settings").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminSettings(sc))
	app.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(superUser).RouteHandler(makeClearTaskQueueHandler(sc))
	app.AddRoute("/cost/distro/{distro_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeCostByDistroHandler(sc))
	app.AddRoute("/cost/version/{version_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeCostByVersionHandler(sc))
	app.AddRoute("/cost/project/{project_id}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeTaskCostByProjectRoute(sc))
	app.AddRoute("/alias/{name}").Version(2).Get().RouteHandler(makeFetchAliases(sc))
	app.AddRoute("/builds/{build_id}").Version(2).Get().RouteHandler(makeGetBuildByID(sc))
	app.AddRoute("/builds/{build_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangeStatusForBuild(sc))
	app.AddRoute("/builds/{build_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeAbortBuild(sc))
	app.AddRoute("/builds/{build_id}/restart").Version(2).Post().Wrap(checkUser).RouteHandler(makeRestartBuild(sc))
	app.AddRoute("/builds/{build_id}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchTasksByBuild(sc))
	app.AddRoute("/hosts").Version(2).Get().RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/hosts").Version(2).Post().Wrap(checkUser).RouteHandler(makeSpawnHostCreateRoute(sc))
	app.AddRoute("/hosts/{host_id}").Version(2).Get().RouteHandler(makeGetHostByID(sc))
	app.AddRoute("/hosts/{task_id}/create").Version(2).Post().RouteHandler(makeHostCreateRouteManager(sc))
	app.AddRoute("/hosts/{task_id}/list").Version(2).Get().RouteHandler(makeHostListRouteManager(sc))
	app.AddRoute("/patches/{patch_id}").Version(2).Get().RouteHandler(makeFetchPatchByID(sc))
	app.AddRoute("/patches/{patch_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangePatchStatus(sc))
	app.AddRoute("/projects/{project_id}/revisions/{commit_hash}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeTasksByProjectAndCommitHandler(sc))
	app.AddRoute("/status/cli_version").Version(2).Get().RouteHandler(makeFetchCLIVersionRoute(sc))
	app.AddRoute("/status/hosts/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeHostStatusByDistroRoute(sc))
	app.AddRoute("/status/notifications").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchNotifcationStatusRoute(sc))
	app.AddRoute("/status/recent_tasks").Version(2).Get().RouteHandler(makeRecentTaskStatusHandler(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeModifyTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeTaskAbortHandler(sc))
	app.AddRoute("/tasks/{task_id}/generate").Version(2).Post().RouteHandler(makeGenerateTasksHandler(sc))
	app.AddRoute("/tasks/{task_id}/metrics/process").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchTaskProcessMetrics(sc))
	app.AddRoute("/tasks/{task_id}/metrics/system").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchTaskSystmMetrics(sc))
	app.AddRoute("/tasks/{task_id}/restart").Version(2).Post().Wrap(checkUser).RouteHandler(makeTaskRestartHandler(sc))
	app.AddRoute("/users/{user_id}/hosts").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/users/{user_id}/patches").Version(2).Get().Wrap(checkUser).RouteHandler(makeUserPatchHandler())
	app.AddRoute("/versions/{version_id}").Version(2).Get().RouteHandler(makeGetVersionByID(sc))
	app.AddRoute("/versions/{version_id}/builds").Version(2).Get().RouteHandler(makeGetVersionBuilds(sc))
}
