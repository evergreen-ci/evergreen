package route

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
)

const defaultLimit = 100

type HandlerOpts struct {
	App               *gimlet.APIApp
	APIQueue          amboy.Queue
	SingleWorkerQueue amboy.Queue
	URL               string
	SuperUsers        []string
	GithubSecret      []byte
}

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(opts HandlerOpts) {
	sc := &data.DBConnector{}

	sc.SetURL(opts.URL)
	sc.SetSuperUsers(opts.SuperUsers)

	// Middleware
	superUser := gimlet.NewRestrictAccessToUsers(sc.GetSuperUsers())
	checkUser := gimlet.NewRequireAuthHandler()
	addProject := NewProjectContextMiddleware(sc)
	checkProjectAdmin := NewProjectAdminMiddleware(sc)

	env := evergreen.GetEnvironment()
	settings := env.Settings()

	// Support override for testing
	if opts.SingleWorkerQueue == nil {
		opts.SingleWorkerQueue = env.SingleWorkerQueue()
	}

	// Routes
	opts.App.AddRoute("/").Version(2).Get().RouteHandler(makePlaceHolderManger(sc))
	opts.App.AddRoute("/admin").Version(2).Get().RouteHandler(makeLegacyAdminConfig(sc))
	opts.App.AddRoute("/admin/banner").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchAdminBanner(sc))
	opts.App.AddRoute("/admin/banner").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminBanner(sc))
	opts.App.AddRoute("/admin/events").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminEvents(sc))
	opts.App.AddRoute("/admin/restart").Version(2).Post().Wrap(superUser).RouteHandler(makeRestartRoute(sc, opts.APIQueue))
	opts.App.AddRoute("/admin/revert").Version(2).Post().Wrap(superUser).RouteHandler(makeRevertRouteManager(sc))
	opts.App.AddRoute("/admin/service_flags").Version(2).Post().Wrap(superUser).RouteHandler(makeSetServiceFlagsRouteManager(sc))
	opts.App.AddRoute("/admin/settings").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminSettings(sc))
	opts.App.AddRoute("/admin/settings").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminSettings(sc))
	opts.App.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(superUser).RouteHandler(makeClearTaskQueueHandler(sc))
	opts.App.AddRoute("/alias/{name}").Version(2).Get().RouteHandler(makeFetchAliases(sc))
	opts.App.AddRoute("/builds/{build_id}").Version(2).Get().RouteHandler(makeGetBuildByID(sc))
	opts.App.AddRoute("/builds/{build_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangeStatusForBuild(sc))
	opts.App.AddRoute("/builds/{build_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeAbortBuild(sc))
	opts.App.AddRoute("/builds/{build_id}/restart").Version(2).Post().Wrap(checkUser).RouteHandler(makeRestartBuild(sc))
	opts.App.AddRoute("/builds/{build_id}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchTasksByBuild(sc))
	opts.App.AddRoute("/cost/distro/{distro_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeCostByDistroHandler(sc))
	opts.App.AddRoute("/cost/project/{project_id}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeTaskCostByProjectRoute(sc))
	opts.App.AddRoute("/cost/version/{version_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeCostByVersionHandler(sc))
	opts.App.AddRoute("/distros").Version(2).Get().Wrap(superUser).RouteHandler(makeDistroRoute(sc))
	opts.App.AddRoute("/distros/{distro_id}").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroByID(sc))
	opts.App.AddRoute("/distros/{distro_id}").Version(2).Patch().Wrap(superUser).RouteHandler(makePatchDistroByID(sc, settings))
	opts.App.AddRoute("/distros/{distro_id}").Version(2).Delete().Wrap(superUser).RouteHandler(makeDeleteDistroByID(sc))
	opts.App.AddRoute("/distros/{distro_id}").Version(2).Put().Wrap(superUser).RouteHandler(makePutDistro(sc, settings))
	opts.App.AddRoute("/distros/{distro_id}/setup").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroSetup(sc))
	opts.App.AddRoute("/distros/{distro_id}/setup").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeDistroSetup(sc))
	opts.App.AddRoute("/distros/{distro_id}/teardown").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroTeardown(sc))
	opts.App.AddRoute("/distros/{distro_id}/teardown").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeDistroTeardown(sc))
	opts.App.AddRoute("/hooks/github").Version(2).Post().RouteHandler(makeGithubHooksRoute(sc, opts.APIQueue, opts.GithubSecret))
	opts.App.AddRoute("/hosts").Version(2).Get().RouteHandler(makeFetchHosts(sc))
	opts.App.AddRoute("/hosts").Version(2).Post().Wrap(checkUser).RouteHandler(makeSpawnHostCreateRoute(sc))
	opts.App.AddRoute("/hosts").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeHostsStatuses(sc))
	opts.App.AddRoute("/hosts/{host_id}").Version(2).Get().RouteHandler(makeGetHostByID(sc))
	opts.App.AddRoute("/hosts/{host_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangeHostStatus(sc))
	opts.App.AddRoute("/hosts/{host_id}/change_password").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostChangePassword(sc))
	opts.App.AddRoute("/hosts/{host_id}/extend_expiration").Version(2).Post().Wrap(checkUser).RouteHandler(makeExtendHostExpiration(sc))
	opts.App.AddRoute("/hosts/{host_id}/terminate").Version(2).Post().Wrap(checkUser).RouteHandler(makeTerminateHostRoute(sc))
	opts.App.AddRoute("/hosts/{task_id}/create").Version(2).Post().RouteHandler(makeHostCreateRouteManager(sc))
	opts.App.AddRoute("/hosts/{task_id}/list").Version(2).Get().RouteHandler(makeHostListRouteManager(sc))
	opts.App.AddRoute("/keys").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchKeys(sc))
	opts.App.AddRoute("/keys").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetKey(sc))
	opts.App.AddRoute("/keys/{key_name}").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteKeys(sc))
	opts.App.AddRoute("/patches/{patch_id}").Version(2).Get().RouteHandler(makeFetchPatchByID(sc))
	opts.App.AddRoute("/patches/{patch_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangePatchStatus(sc))
	opts.App.AddRoute("/patches/{patch_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeAbortPatch(sc))
	opts.App.AddRoute("/patches/{patch_id}/restart").Version(2).Post().Wrap(checkUser).RouteHandler(makeRestartPatch(sc))
	opts.App.AddRoute("/projects").Version(2).Get().RouteHandler(makeFetchProjectsRoute(sc))
	opts.App.AddRoute("/projects/{project_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectByID(sc))
	opts.App.AddRoute("/projects/{project_id}").Version(2).Put().Wrap(superUser).RouteHandler(makePutProjectByID(sc))
	opts.App.AddRoute("/projects/{project_id}").Version(2).Patch().Wrap(superUser).RouteHandler(makePatchProjectByID(sc))
	opts.App.AddRoute("/projects/{project_id}/events").Version(2).Get().Wrap(checkUser, addProject, checkProjectAdmin).RouteHandler(makeFetchProjectEvents(sc))
	opts.App.AddRoute("/projects/{project_id}/patches").Version(2).Get().Wrap(checkUser).RouteHandler(makePatchesByProjectRoute(sc))
	opts.App.AddRoute("/projects/{project_id}/versions/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchProjectTasks(sc))
	opts.App.AddRoute("/projects/{project_id}/recent_versions").Version(2).Get().RouteHandler(makeFetchProjectVersions(sc))
	opts.App.AddRoute("/projects/{project_id}/revisions/{commit_hash}/tasks").Version(2).Get().Wrap(checkUser).RouteHandler(makeTasksByProjectAndCommitHandler(sc))
	opts.App.AddRoute("/projects/{project_id}/test_stats").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectTestStats(sc))
	opts.App.AddRoute("/projects/{project_id}/task_stats").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectTaskStats(sc))
	opts.App.AddRoute("/projects/{project_id}/commit_queue").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetCommitQueueItems(sc))
	opts.App.AddRoute("/status/cli_version").Version(2).Get().RouteHandler(makeFetchCLIVersionRoute(sc))
	opts.App.AddRoute("/status/hosts/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeHostStatusByDistroRoute(sc))
	opts.App.AddRoute("/status/notifications").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchNotifcationStatusRoute(sc))
	opts.App.AddRoute("/status/recent_tasks").Version(2).Get().RouteHandler(makeRecentTaskStatusHandler(sc))
	opts.App.AddRoute("/subscriptions").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteSubscription(sc))
	opts.App.AddRoute("/subscriptions").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchSubscription(sc))
	opts.App.AddRoute("/subscriptions").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetSubscrition(sc))
	opts.App.AddRoute("/tasks/{task_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetTaskRoute(sc))
	opts.App.AddRoute("/tasks/{task_id}").Version(2).Patch().Wrap(checkUser, addProject).RouteHandler(makeModifyTaskRoute(sc))
	opts.App.AddRoute("/tasks/{task_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeTaskAbortHandler(sc))
	opts.App.AddRoute("/tasks/{task_id}/generate").Version(2).Post().RouteHandler(makeGenerateTasksHandler(sc))
	opts.App.AddRoute("/tasks/{task_id}/generate").Version(2).Get().RouteHandler(makeGenerateTasksPollHandler(sc, opts.SingleWorkerQueue))
	opts.App.AddRoute("/tasks/{task_id}/restart").Version(2).Post().Wrap(addProject, checkUser).RouteHandler(makeTaskRestartHandler(sc))
	opts.App.AddRoute("/tasks/{task_id}/tests").Version(2).Get().Wrap(addProject).RouteHandler(makeFetchTestsForTask(sc))
	opts.App.AddRoute("/user/settings").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchUserConfig())
	opts.App.AddRoute("/user/settings").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetUserConfig(sc))
	opts.App.AddRoute("/users/{user_id}/hosts").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchHosts(sc))
	opts.App.AddRoute("/users/{user_id}/patches").Version(2).Get().Wrap(checkUser).RouteHandler(makeUserPatchHandler(sc))
	opts.App.AddRoute("/versions").Version(2).Put().RouteHandler(makeVersionCreateHandler(sc))
	opts.App.AddRoute("/versions/{version_id}").Version(2).Get().RouteHandler(makeGetVersionByID(sc))
	opts.App.AddRoute("/versions/{version_id}/abort").Version(2).Post().Wrap(checkUser).RouteHandler(makeAbortVersion(sc))
	opts.App.AddRoute("/versions/{version_id}/builds").Version(2).Get().RouteHandler(makeGetVersionBuilds(sc))
	opts.App.AddRoute("/versions/{version_id}/restart").Version(2).Post().Wrap(checkUser).RouteHandler(makeRestartVersion(sc))
}
