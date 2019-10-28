package route

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/acl"
	"github.com/mongodb/amboy"
)

const defaultLimit = 100

type HandlerOpts struct {
	APIQueue     amboy.Queue
	QueueGroup   amboy.QueueGroup
	URL          string
	SuperUsers   []string
	GithubSecret []byte
}

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(app *gimlet.APIApp, opts HandlerOpts) {
	sc := &data.DBConnector{}

	sc.SetURL(opts.URL)
	sc.SetSuperUsers(opts.SuperUsers)

	// Middleware
	superUser := gimlet.NewRestrictAccessToUsers(sc.GetSuperUsers())
	checkUser := gimlet.NewRequireAuthHandler()
	checkTask := NewTaskAuthMiddleware(sc)
	addProject := NewProjectContextMiddleware(sc)
	checkProjectAdmin := NewProjectAdminMiddleware(sc)
	checkCommitQueueItemOwner := NewCommitQueueItemOwnerMiddleware(sc)
	viewTasks := &RequiresProjectViewPermission{}
	editTasks := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksBasic)
	submitPatches := RequiresProjectPermission(evergreen.PermissionPatches, evergreen.PatchSubmit)
	viewProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView)
	editProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsEdit)

	env := evergreen.GetEnvironment()
	settings := env.Settings()

	// Routes
	app.AddRoute("/").Version(2).Get().RouteHandler(makePlaceHolderManger(sc))
	app.AddRoute("/admin").Version(2).Get().RouteHandler(makeLegacyAdminConfig(sc))
	app.AddRoute("/admin/banner").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchAdminBanner(sc))
	app.AddRoute("/admin/banner").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminBanner(sc))
	app.AddRoute("/admin/bugsnag").Version(2).Get().RouteHandler(makeFetchBugsnag(sc))
	app.AddRoute("/admin/events").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminEvents(sc))
	app.AddRoute("/admin/restart/versions").Version(2).Post().Wrap(superUser).RouteHandler(makeRestartRoute(sc, evergreen.RestartVersions, nil))
	app.AddRoute("/admin/restart/tasks").Version(2).Post().Wrap(superUser).RouteHandler(makeRestartRoute(sc, evergreen.RestartTasks, opts.APIQueue))
	app.AddRoute("/admin/revert").Version(2).Post().Wrap(superUser).RouteHandler(makeRevertRouteManager(sc))
	app.AddRoute("/admin/service_flags").Version(2).Post().Wrap(superUser).RouteHandler(makeSetServiceFlagsRouteManager(sc))
	app.AddRoute("/admin/settings").Version(2).Get().Wrap(superUser).RouteHandler(makeFetchAdminSettings(sc))
	app.AddRoute("/admin/settings").Version(2).Post().Wrap(superUser).RouteHandler(makeSetAdminSettings(sc))
	app.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(superUser).RouteHandler(makeClearTaskQueueHandler(sc))
	app.AddRoute("/admin/commit_queues").Version(2).Delete().Wrap(superUser).RouteHandler(makeClearCommitQueuesHandler(sc))
	app.AddRoute("/alias/{name}").Version(2).Get().RouteHandler(makeFetchAliases(sc))
	app.AddRoute("/builds/{build_id}").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetBuildByID(sc))
	app.AddRoute("/builds/{build_id}").Version(2).Patch().Wrap(checkUser, editTasks).RouteHandler(makeChangeStatusForBuild(sc))
	app.AddRoute("/builds/{build_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeAbortBuild(sc))
	app.AddRoute("/builds/{build_id}/restart").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeRestartBuild(sc))
	app.AddRoute("/builds/{build_id}/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchTasksByBuild(sc))
	app.AddRoute("/commit_queue/{project_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetCommitQueueItems(sc))
	app.AddRoute("/commit_queue/{project_id}/{item}").Version(2).Delete().Wrap(checkUser, addProject, checkCommitQueueItemOwner, editTasks).RouteHandler(makeDeleteCommitQueueItems(sc, env))
	app.AddRoute("/commit_queue/{patch_id}").Version(2).Put().Wrap(checkUser, addProject, checkCommitQueueItemOwner, editTasks).RouteHandler(makeCommitQueueEnqueueItem(sc))
	app.AddRoute("/commit_queue/{project_id}/{item}/author").Version(2).Get().Wrap(checkUser, addProject, viewTasks).RouteHandler(makeGetCommitQueueItemAuthor(sc))
	app.AddRoute("/cost/distro/{distro_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeCostByDistroHandler(sc))
	app.AddRoute("/cost/project/{project_id}/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeTaskCostByProjectRoute(sc))
	app.AddRoute("/cost/version/{version_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeCostByVersionHandler(sc))
	app.AddRoute("/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeDistroRoute(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroByID(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Patch().Wrap(superUser).RouteHandler(makePatchDistroByID(sc, settings))
	app.AddRoute("/distros/{distro_id}").Version(2).Delete().Wrap(superUser).RouteHandler(makeDeleteDistroByID(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Put().Wrap(superUser).RouteHandler(makePutDistro(sc, settings))
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroSetup(sc))
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeDistroSetup(sc))
	app.AddRoute("/distros/{distro_id}/teardown").Version(2).Get().Wrap(superUser).RouteHandler(makeGetDistroTeardown(sc))
	app.AddRoute("/distros/{distro_id}/teardown").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeDistroTeardown(sc))
	app.AddRoute("/distros/{distro_id}/execute").Version(2).Post().Wrap(superUser).RouteHandler(makeDistroExecute(sc, env))
	app.AddRoute("/hooks/github").Version(2).Post().RouteHandler(makeGithubHooksRoute(sc, opts.APIQueue, opts.GithubSecret, settings))
	app.AddRoute("/hosts").Version(2).Get().RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/hosts").Version(2).Post().Wrap(checkUser).RouteHandler(makeSpawnHostCreateRoute(sc, env.Settings()))
	app.AddRoute("/hosts").Version(2).Patch().Wrap(superUser).RouteHandler(makeChangeHostsStatuses(sc))
	app.AddRoute("/hosts/{host_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetHostByID(sc))
	app.AddRoute("/hosts/{host_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeHostModifyRouteManager(sc, env))
	app.AddRoute("/hosts/{host_id}/stop").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostStopManager(sc, env))
	app.AddRoute("/hosts/{host_id}/start").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostStartManager(sc, env))
	app.AddRoute("/hosts/{host_id}/change_password").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostChangePassword(sc, env))
	app.AddRoute("/hosts/{host_id}/extend_expiration").Version(2).Post().Wrap(checkUser).RouteHandler(makeExtendHostExpiration(sc))
	app.AddRoute("/hosts/{host_id}/terminate").Version(2).Post().Wrap(checkUser).RouteHandler(makeTerminateHostRoute(sc))
	app.AddRoute("/hosts/{host_id}/status").Version(2).Get().RouteHandler(makeContainerStatusManager(sc))
	app.AddRoute("/hosts/{host_id}/logs/output").Version(2).Get().RouteHandler(makeContainerLogsRouteManager(sc, false))
	app.AddRoute("/hosts/{host_id}/logs/error").Version(2).Get().RouteHandler(makeContainerLogsRouteManager(sc, true))
	app.AddRoute("/hosts/{task_id}/create").Version(2).Post().Wrap(checkTask).RouteHandler(makeHostCreateRouteManager(sc))
	app.AddRoute("/hosts/{task_id}/list").Version(2).Get().Wrap(checkTask).RouteHandler(makeHostListRouteManager(sc))
	app.AddRoute("/hosts/{host_id}/attach").Version(2).Post().Wrap(checkUser).RouteHandler(makeAttachVolume(sc, env))
	app.AddRoute("/hosts/{host_id}/detach").Version(2).Post().Wrap(checkUser).RouteHandler(makeDetachVolume(sc, env))
	app.AddRoute("/volumes").Version(2).Post().Wrap(checkUser).RouteHandler(makeCreateVolume(sc, env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Wrap(checkUser).Delete().RouteHandler(makeDeleteVolume(sc, env))
	app.AddRoute("/keys").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchKeys(sc))
	app.AddRoute("/keys").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetKey(sc))
	app.AddRoute("/keys/{key_name}").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteKeys(sc))
	app.AddRoute("/notifications/{type}").Version(2).Post().Wrap(checkUser).RouteHandler(makeNotification(env))
	app.AddRoute("/patches/{patch_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchPatchByID(sc))
	app.AddRoute("/patches/{patch_id}").Version(2).Patch().Wrap(checkUser, submitPatches).RouteHandler(makeChangePatchStatus(sc))
	app.AddRoute("/patches/{patch_id}/abort").Version(2).Post().Wrap(checkUser, submitPatches).RouteHandler(makeAbortPatch(sc))
	app.AddRoute("/patches/{patch_id}/restart").Version(2).Post().Wrap(checkUser, submitPatches).RouteHandler(makeRestartPatch(sc))
	app.AddRoute("/projects").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchProjectsRoute(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Put().Wrap(superUser).RouteHandler(makePutProjectByID(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Get().Wrap(checkUser, addProject, checkProjectAdmin, viewProjectSettings).RouteHandler(makeGetProjectByID(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Patch().Wrap(checkUser, addProject, checkProjectAdmin, editProjectSettings).RouteHandler(makePatchProjectByID(sc))
	app.AddRoute("/projects/{project_id}/copy").Version(2).Post().Wrap(checkUser, addProject, checkProjectAdmin, editProjectSettings).RouteHandler(makeCopyProject(sc))
	app.AddRoute("/projects/{project_id}/events").Version(2).Get().Wrap(checkUser, addProject, checkProjectAdmin, viewProjectSettings).RouteHandler(makeFetchProjectEvents(sc))
	app.AddRoute("/projects/{project_id}/patches").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makePatchesByProjectRoute(sc))
	app.AddRoute("/projects/{project_id}/versions").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetProjectVersionsHandler(sc))
	app.AddRoute("/projects/{project_id}/versions/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchProjectTasks(sc))
	app.AddRoute("/projects/{project_id}/recent_versions").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchProjectVersions(sc))
	app.AddRoute("/projects/{project_id}/revisions/{commit_hash}/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeTasksByProjectAndCommitHandler(sc))
	app.AddRoute("/projects/{project_id}/test_stats").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetProjectTestStats(sc))
	app.AddRoute("/projects/{project_id}/task_stats").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetProjectTaskStats(sc))
	app.AddRoute("/projects/{project_id}/task_reliability").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectTaskReliability(sc))
	app.AddRoute("/roles").Version(2).Get().Wrap(checkUser).RouteHandler(acl.NewGetAllRolesHandler(env.RoleManager()))
	app.AddRoute("/roles").Version(2).Post().Wrap(checkUser).RouteHandler(acl.NewUpdateRoleHandler(env.RoleManager()))
	app.AddRoute("/status/cli_version").Version(2).Get().RouteHandler(makeFetchCLIVersionRoute(sc))
	app.AddRoute("/status/hosts/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeHostStatusByDistroRoute(sc))
	app.AddRoute("/status/notifications").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchNotifcationStatusRoute(sc))
	app.AddRoute("/status/recent_tasks").Version(2).Get().RouteHandler(makeRecentTaskStatusHandler(sc))
	app.AddRoute("/subscriptions").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteSubscription(sc))
	app.AddRoute("/subscriptions").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchSubscription(sc))
	app.AddRoute("/subscriptions").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetSubscription(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Patch().Wrap(checkUser, addProject, editTasks).RouteHandler(makeModifyTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeTaskAbortHandler(sc))
	app.AddRoute("/tasks/{task_id}/generate").Version(2).Post().Wrap(checkTask).RouteHandler(makeGenerateTasksHandler(sc, opts.QueueGroup))
	app.AddRoute("/tasks/{task_id}/generate").Version(2).Get().Wrap(checkTask).RouteHandler(makeGenerateTasksPollHandler(sc, opts.QueueGroup))
	app.AddRoute("/tasks/{task_id}/manifest").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetManifestHandler(sc))
	app.AddRoute("/tasks/{task_id}/restart").Version(2).Post().Wrap(addProject, checkUser, editTasks).RouteHandler(makeTaskRestartHandler(sc))
	app.AddRoute("/tasks/{task_id}/tests").Version(2).Get().Wrap(addProject, viewTasks).RouteHandler(makeFetchTestsForTask(sc))
	app.AddRoute("/user/settings").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchUserConfig())
	app.AddRoute("/user/settings").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetUserConfig(sc))
	app.AddRoute("/user/author/{user_id}").Version(2).Get().Wrap(checkTask).RouteHandler(makeFetchUserAuthor(sc))
	app.AddRoute("/users/{user_id}/hosts").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/users/{user_id}/patches").Version(2).Get().Wrap(checkUser).RouteHandler(makeUserPatchHandler(sc))
	app.AddRoute("/versions").Version(2).Put().RouteHandler(makeVersionCreateHandler(sc))
	app.AddRoute("/versions/{version_id}").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetVersionByID(sc))
	app.AddRoute("/versions/{version_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeAbortVersion(sc))
	app.AddRoute("/versions/{version_id}/builds").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetVersionBuilds(sc))
	app.AddRoute("/versions/{version_id}/restart").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeRestartVersion(sc))
}
