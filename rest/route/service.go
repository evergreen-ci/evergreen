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
	GithubSecret []byte
}

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(app *gimlet.APIApp, opts HandlerOpts) {
	sc := &data.DBConnector{}
	env := evergreen.GetEnvironment()
	settings := env.Settings()

	sc.SetURL(opts.URL)

	// Middleware
	checkUser := gimlet.NewRequireAuthHandler()
	checkTask := NewTaskAuthMiddleware(sc)
	checkTaskHost := NewTaskHostAuthMiddleware(sc)
	checkHost := NewHostAuthMiddleware(sc)
	checkPod := NewPodAuthMiddleware(sc)
	addProject := NewProjectContextMiddleware(sc)
	checkProjectAdmin := NewProjectAdminMiddleware(sc)
	checkRepoAdmin := NewRepoAdminMiddleware(sc)
	checkCommitQueueItemOwner := NewCommitQueueItemOwnerMiddleware(sc)
	adminSettings := RequiresSuperUserPermission(evergreen.PermissionAdminSettings, evergreen.AdminSettingsEdit)
	createProject := RequiresSuperUserPermission(evergreen.PermissionProjectCreate, evergreen.ProjectCreate)
	createDistro := RequiresSuperUserPermission(evergreen.PermissionDistroCreate, evergreen.DistroCreate)
	editRoles := RequiresSuperUserPermission(evergreen.PermissionRoleModify, evergreen.RoleModify)
	viewTasks := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	editTasks := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksBasic)
	editAnnotations := RequiresProjectPermission(evergreen.PermissionAnnotations, evergreen.AnnotationsModify)
	viewAnnotations := RequiresProjectPermission(evergreen.PermissionAnnotations, evergreen.AnnotationsView)
	submitPatches := RequiresProjectPermission(evergreen.PermissionPatches, evergreen.PatchSubmit)
	viewProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView)
	editProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsEdit)
	editDistroSettings := RequiresDistroPermission(evergreen.PermissionDistroSettings, evergreen.DistroSettingsEdit)
	removeDistroSettings := RequiresDistroPermission(evergreen.PermissionDistroSettings, evergreen.DistroSettingsAdmin)
	editHosts := RequiresDistroPermission(evergreen.PermissionHosts, evergreen.HostsEdit)
	cedarTestStats := checkCedarTestStats(settings)

	app.AddWrapper(gimlet.WrapperMiddleware(allowCORS))

	// Routes
	app.AddRoute("/").Version(2).Get().RouteHandler(makePlaceHolderManger(sc))
	app.AddRoute("/admin/banner").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchAdminBanner(sc))
	app.AddRoute("/admin/banner").Version(2).Post().Wrap(adminSettings).RouteHandler(makeSetAdminBanner(sc))
	app.AddRoute("/admin/events").Version(2).Get().Wrap(adminSettings).RouteHandler(makeFetchAdminEvents(sc))
	app.AddRoute("/admin/spawn_hosts").Version(2).Get().Wrap(adminSettings).RouteHandler(makeFetchSpawnHostUsage(sc))
	app.AddRoute("/admin/restart/versions").Version(2).Post().Wrap(adminSettings).RouteHandler(makeRestartRoute(sc, evergreen.RestartVersions, nil))
	app.AddRoute("/admin/restart/tasks").Version(2).Post().Wrap(adminSettings).RouteHandler(makeRestartRoute(sc, evergreen.RestartTasks, opts.APIQueue))
	app.AddRoute("/admin/revert").Version(2).Post().Wrap(adminSettings).RouteHandler(makeRevertRouteManager(sc))
	app.AddRoute("/admin/service_flags").Version(2).Post().Wrap(adminSettings).RouteHandler(makeSetServiceFlagsRouteManager(sc))
	app.AddRoute("/admin/settings").Version(2).Get().Wrap(adminSettings).RouteHandler(makeFetchAdminSettings(sc))
	app.AddRoute("/admin/settings").Version(2).Post().Wrap(adminSettings).RouteHandler(makeSetAdminSettings(sc))
	app.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(adminSettings).RouteHandler(makeClearTaskQueueHandler(sc))
	app.AddRoute("/admin/commit_queues").Version(2).Delete().Wrap(adminSettings).RouteHandler(makeClearCommitQueuesHandler(sc))
	app.AddRoute("/admin/service_users").Version(2).Get().Wrap(adminSettings).RouteHandler(makeGetServiceUsers(sc))
	app.AddRoute("/admin/service_users").Version(2).Post().Wrap(adminSettings).RouteHandler(makeUpdateServiceUser(sc))
	app.AddRoute("/admin/service_users").Version(2).Delete().Wrap(adminSettings).RouteHandler(makeDeleteServiceUser(sc))
	app.AddRoute("/alias/{name}").Version(2).Get().RouteHandler(makeFetchAliases(sc))
	app.AddRoute("/auth").Version(2).Get().Wrap(checkUser).RouteHandler(&authPermissionGetHandler{})
	app.AddRoute("/builds/{build_id}").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetBuildByID(sc))
	app.AddRoute("/builds/{build_id}").Version(2).Patch().Wrap(checkUser, editTasks).RouteHandler(makeChangeStatusForBuild(sc))
	app.AddRoute("/builds/{build_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeAbortBuild(sc))
	app.AddRoute("/builds/{build_id}/restart").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeRestartBuild(sc))
	app.AddRoute("/builds/{build_id}/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchTasksByBuild(sc))
	app.AddRoute("/builds/{build_id}/annotations").Version(2).Get().Wrap(checkUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByBuild(sc))
	app.AddRoute("/commit_queue/{project_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetCommitQueueItems(sc))
	app.AddRoute("/commit_queue/{project_id}/{item}").Version(2).Delete().Wrap(checkUser, addProject, checkCommitQueueItemOwner, editTasks).RouteHandler(makeDeleteCommitQueueItems(sc, env))
	app.AddRoute("/commit_queue/{patch_id}").Version(2).Put().Wrap(checkUser, addProject, checkCommitQueueItemOwner, editTasks).RouteHandler(makeCommitQueueEnqueueItem(sc))
	app.AddRoute("/commit_queue/{patch_id}/additional").Version(2).Get().Wrap(checkTask).RouteHandler(makeCommitQueueAdditionalPatches(sc))
	app.AddRoute("/commit_queue/{patch_id}/conclude_merge").Version(2).Post().Wrap(checkTask).RouteHandler(makeCommitQueueConcludeMerge(sc))
	app.AddRoute("/commit_queue/{patch_id}/message").Version(2).Get().Wrap(checkUser).RouteHandler(makecqMessageForPatch(sc))
	app.AddRoute("/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeDistroRoute(sc))
	app.AddRoute("/distros/settings").Version(2).Patch().Wrap(createDistro).RouteHandler(makeModifyDistrosSettings(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Get().Wrap(editDistroSettings).RouteHandler(makeGetDistroByID(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Patch().Wrap(editDistroSettings).RouteHandler(makePatchDistroByID(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Delete().Wrap(removeDistroSettings).RouteHandler(makeDeleteDistroByID(sc))
	app.AddRoute("/distros/{distro_id}").Version(2).Put().Wrap(createDistro).RouteHandler(makePutDistro(sc))
	app.AddRoute("/distros/{distro_id}/ami").Version(2).Get().Wrap(checkTask).RouteHandler(makeGetDistroAMI(sc))
	app.AddRoute("/distros/{distro_id}/client_urls").Version(2).Get().RouteHandler(makeGetDistroClientURLs(sc, env))
	app.AddRoute("/distros/{distro_id}/execute").Version(2).Patch().Wrap(editHosts).RouteHandler(makeDistroExecute(sc, env))
	app.AddRoute("/distros/{distro_id}/icecream_config").Version(2).Patch().Wrap(editHosts).RouteHandler(makeDistroIcecreamConfig(sc, env))
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Get().Wrap(editDistroSettings).RouteHandler(makeGetDistroSetup(sc))
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Patch().Wrap(editDistroSettings).RouteHandler(makeChangeDistroSetup(sc))

	app.AddRoute("/hooks/github").Version(2).Post().RouteHandler(makeGithubHooksRoute(sc, opts.APIQueue, opts.GithubSecret, settings))
	app.AddRoute("/hooks/aws").Version(2).Post().RouteHandler(makeEC2SNS(sc, env, opts.APIQueue))
	app.AddRoute("/hooks/aws/ecs").Version(2).Post().RouteHandler(makeECSSNS(sc, env, opts.APIQueue))
	app.AddRoute("/host/filter").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchHostFilter(sc))
	app.AddRoute("/host/start_processes").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostStartProcesses(sc, env))
	app.AddRoute("/host/get_processes").Version(2).Get().Wrap(checkUser).RouteHandler(makeHostGetProcesses(sc, env))
	app.AddRoute("/hosts").Version(2).Get().RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/hosts").Version(2).Post().Wrap(checkUser).RouteHandler(makeSpawnHostCreateRoute(sc, env.Settings()))
	app.AddRoute("/hosts").Version(2).Patch().Wrap(checkUser).RouteHandler(makeChangeHostsStatuses(sc))
	app.AddRoute("/hosts/{host_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetHostByID(sc))
	app.AddRoute("/hosts/{host_id}").Version(2).Patch().Wrap(checkUser).RouteHandler(makeHostModifyRouteManager(sc, env))
	app.AddRoute("/hosts/{host_id}/disable").Version(2).Post().Wrap(checkHost).RouteHandler(makeDisableHostHandler(sc, env))
	app.AddRoute("/hosts/{host_id}/stop").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostStopManager(sc, env))
	app.AddRoute("/hosts/{host_id}/start").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostStartManager(sc, env))
	app.AddRoute("/hosts/{host_id}/change_password").Version(2).Post().Wrap(checkUser).RouteHandler(makeHostChangePassword(sc, env))
	app.AddRoute("/hosts/{host_id}/extend_expiration").Version(2).Post().Wrap(checkUser).RouteHandler(makeExtendHostExpiration(sc))
	app.AddRoute("/hosts/{host_id}/terminate").Version(2).Post().Wrap(checkUser).RouteHandler(makeTerminateHostRoute(sc))
	app.AddRoute("/hosts/{host_id}/status").Version(2).Get().Wrap(checkTaskHost).RouteHandler(makeContainerStatusManager(sc))
	app.AddRoute("/hosts/{host_id}/logs/output").Version(2).Get().Wrap(checkTaskHost).RouteHandler(makeContainerLogsRouteManager(sc, false))
	app.AddRoute("/hosts/{host_id}/logs/error").Version(2).Get().Wrap(checkTaskHost).RouteHandler(makeContainerLogsRouteManager(sc, true))
	app.AddRoute("/hosts/{task_id}/create").Version(2).Post().Wrap(checkTask).RouteHandler(makeHostCreateRouteManager(sc))
	app.AddRoute("/hosts/{task_id}/list").Version(2).Get().Wrap(checkTask).RouteHandler(makeHostListRouteManager(sc))
	app.AddRoute("/hosts/{host_id}/attach").Version(2).Post().Wrap(checkUser).RouteHandler(makeAttachVolume(sc, env))
	app.AddRoute("/hosts/{host_id}/detach").Version(2).Post().Wrap(checkUser).RouteHandler(makeDetachVolume(sc, env))
	app.AddRoute("/hosts/{host_id}/provisioning_options").Version(2).Get().Wrap(checkHost).RouteHandler(makeHostProvisioningOptionsGetHandler(sc))
	app.AddRoute("/hosts/ip_address/{ip_address}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetHostByIpAddress(sc))
	app.AddRoute("/volumes").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetVolumes(sc))
	app.AddRoute("/volumes").Version(2).Post().Wrap(checkUser).RouteHandler(makeCreateVolume(sc, env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Wrap(checkUser).Delete().RouteHandler(makeDeleteVolume(sc, env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Wrap(checkUser).Patch().RouteHandler(makeModifyVolume(sc, env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetVolumeByID(sc))
	app.AddRoute("/keys").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchKeys(sc))
	app.AddRoute("/keys").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetKey(sc))
	app.AddRoute("/keys/{key_name}").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteKeys(sc))
	app.AddRoute("/notifications/{type}").Version(2).Post().Wrap(checkUser).RouteHandler(makeNotification(env))
	app.AddRoute("/patches/{patch_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchPatchByID(sc))
	app.AddRoute("/patches/{patch_id}").Version(2).Patch().Wrap(checkUser, submitPatches).RouteHandler(makeChangePatchStatus(sc, env))
	app.AddRoute("/patches/{patch_id}/abort").Version(2).Post().Wrap(checkUser, submitPatches).RouteHandler(makeAbortPatch(sc))
	app.AddRoute("/patches/{patch_id}/configure").Version(2).Post().Wrap(checkUser, submitPatches).RouteHandler(makeSchedulePatchHandler(sc))
	app.AddRoute("/patches/{patch_id}/raw").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makePatchRawHandler(sc))
	app.AddRoute("/patches/{patch_id}/restart").Version(2).Post().Wrap(checkUser, submitPatches).RouteHandler(makeRestartPatch(sc))
	app.AddRoute("/patches/{patch_id}/merge_patch").Version(2).Put().Wrap(checkUser, addProject, submitPatches, checkCommitQueueItemOwner).RouteHandler(makeMergePatch(sc))
	app.AddRoute("/pods/{pod_id}/agent/cedar_config").Version(2).Get().Wrap(checkPod).RouteHandler(makePodAgentCedarConfig(env.Settings()))
	app.AddRoute("/pods/{pod_id}/agent/setup").Version(2).Get().Wrap(checkPod).RouteHandler(makePodAgentSetup(env.Settings()))
	app.AddRoute("/pods/{pod_id}/agent/next_task").Version(2).Get().Wrap(checkPod).RouteHandler(makePodAgentNextTask(env, sc))
	app.AddRoute("/pods").Version(2).Post().Wrap(adminSettings).RouteHandler(makePostPod(env, sc))
	app.AddRoute("/projects").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchProjectsRoute(sc))
	app.AddRoute("/projects/test_alias").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectAliasResultsHandler(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Delete().Wrap(checkUser, checkProjectAdmin, editProjectSettings).RouteHandler(makeDeleteProject(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Get().Wrap(checkUser, addProject, viewProjectSettings).RouteHandler(makeGetProjectByID(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Patch().Wrap(checkUser, addProject, checkProjectAdmin, editProjectSettings).RouteHandler(makePatchProjectByID(sc, env.Settings()))
	app.AddRoute("/projects/{project_id}/repotracker").Version(2).Post().Wrap(checkUser, addProject).RouteHandler(makeRunRepotrackerForProject(sc))
	app.AddRoute("/projects/{project_id}").Version(2).Put().Wrap(createProject).RouteHandler(makePutProjectByID(sc))
	app.AddRoute("/projects/{project_id}/copy").Version(2).Post().Wrap(checkUser, addProject, createProject, checkProjectAdmin, editProjectSettings).RouteHandler(makeCopyProject(sc))
	app.AddRoute("/projects/{project_id}/copy/variables").Version(2).Post().Wrap(checkUser, addProject, checkProjectAdmin, editProjectSettings).RouteHandler(makeCopyVariables(sc))
	app.AddRoute("/projects/{project_id}/events").Version(2).Get().Wrap(checkUser, addProject, checkProjectAdmin, viewProjectSettings).RouteHandler(makeFetchProjectEvents(sc))
	app.AddRoute("/projects/{project_id}/patches").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makePatchesByProjectRoute(sc))
	app.AddRoute("/projects/{project_id}/recent_versions").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchProjectVersionsLegacy(sc))
	app.AddRoute("/projects/{project_id}/revisions/{commit_hash}/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeTasksByProjectAndCommitHandler(sc))
	app.AddRoute("/projects/{project_id}/task_reliability").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetProjectTaskReliability(sc))
	app.AddRoute("/projects/{project_id}/task_stats").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetProjectTaskStats(sc))
	app.AddRoute("/projects/{project_id}/test_stats").Version(2).Get().Wrap(checkUser, viewTasks, cedarTestStats).RouteHandler(makeGetProjectTestStats(sc))
	app.AddRoute("/projects/{project_id}/versions").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetProjectVersionsHandler(sc))
	app.AddRoute("/projects/{project_id}/versions/tasks").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchProjectTasks(sc))
	app.AddRoute("/projects/{project_id}/patch_trigger_aliases").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchPatchTriggerAliases(sc))
	app.AddRoute("/projects/{project_id}/parameters").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeFetchParameters(sc))
	app.AddRoute("/projects/variables/rotate").Version(2).Put().Wrap(checkUser, createProject).RouteHandler(makeProjectVarsPut(sc))
	app.AddRoute("/permissions").Version(2).Get().RouteHandler(&permissionsGetHandler{})
	app.AddRoute("/repos/{repo_id}").Version(2).Get().Wrap(checkUser, viewProjectSettings).RouteHandler(makeGetRepoByID(sc))
	app.AddRoute("/repos/{repo_id}").Version(2).Patch().Wrap(checkUser, checkRepoAdmin, editProjectSettings).RouteHandler(makePatchRepoByID(sc, env.Settings()))
	app.AddRoute("/roles").Version(2).Get().Wrap(checkUser).RouteHandler(acl.NewGetAllRolesHandler(env.RoleManager()))
	app.AddRoute("/roles").Version(2).Post().Wrap(checkUser).RouteHandler(acl.NewUpdateRoleHandler(env.RoleManager()))
	app.AddRoute("/roles/{role_id}/users").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetUsersWithRole(sc))
	app.AddRoute("/scheduler/compare_tasks").Version(2).Post().Wrap(checkUser).RouteHandler(makeCompareTasksRoute(sc))
	app.AddRoute("/status/cli_version").Version(2).Get().RouteHandler(makeFetchCLIVersionRoute(sc))
	app.AddRoute("/status/hosts/distros").Version(2).Get().Wrap(checkUser).RouteHandler(makeHostStatusByDistroRoute(sc))
	app.AddRoute("/status/notifications").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchNotifcationStatusRoute(sc))
	app.AddRoute("/status/recent_tasks").Version(2).Get().RouteHandler(makeRecentTaskStatusHandler(sc))
	app.AddRoute("/subscriptions").Version(2).Delete().Wrap(checkUser).RouteHandler(makeDeleteSubscription(sc))
	app.AddRoute("/subscriptions").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchSubscription(sc))
	app.AddRoute("/subscriptions").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetSubscription(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Get().Wrap(checkUser, viewTasks).RouteHandler(makeGetTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}").Version(2).Patch().Wrap(checkUser, addProject, editTasks).RouteHandler(makeModifyTaskRoute(sc))
	app.AddRoute("/tasks/{task_id}/annotations").Version(2).Get().Wrap(checkUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByTask(sc))
	app.AddRoute("/tasks/{task_id}/annotation").Version(2).Put().Wrap(checkUser, editAnnotations).RouteHandler(makePutAnnotationsByTask(sc))
	app.AddRoute("/tasks/{task_id}/created_ticket").Version(2).Put().Wrap(checkUser, editAnnotations).RouteHandler(makeCreatedTicketByTask(sc))
	app.AddRoute("/tasks/{task_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeTaskAbortHandler(sc))
	app.AddRoute("/tasks/{task_id}/display_task").Version(2).Get().Wrap(checkTask).RouteHandler(makeGetDisplayTaskHandler(sc))
	app.AddRoute("/tasks/{task_id}/generate").Version(2).Post().Wrap(checkTask).RouteHandler(makeGenerateTasksHandler(sc, opts.QueueGroup))
	app.AddRoute("/tasks/{task_id}/generate").Version(2).Get().Wrap(checkTask).RouteHandler(makeGenerateTasksPollHandler(sc, opts.QueueGroup))
	app.AddRoute("/tasks/{task_id}/manifest").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetManifestHandler(sc))
	app.AddRoute("/tasks/{task_id}/restart").Version(2).Post().Wrap(addProject, checkUser, editTasks).RouteHandler(makeTaskRestartHandler(sc))
	app.AddRoute("/tasks/{task_id}/tests").Version(2).Get().Wrap(addProject, viewTasks).RouteHandler(makeFetchTestsForTask(sc))
	app.AddRoute("/tasks/{task_id}/sync_path").Version(2).Get().Wrap(checkUser).RouteHandler(makeTaskSyncPathGetHandler(sc))
	app.AddRoute("/tasks/{task_id}/set_has_cedar_results").Version(2).Post().Wrap(checkTask).RouteHandler(makeTaskSetHasCedarResultsHandler(sc))
	app.AddRoute("/task/sync_read_credentials").Version(2).Get().Wrap(checkUser).RouteHandler(makeTaskSyncReadCredentialsGetHandler(sc))
	app.AddRoute("/user/settings").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchUserConfig())
	app.AddRoute("/user/settings").Version(2).Post().Wrap(checkUser).RouteHandler(makeSetUserConfig(sc))
	app.AddRoute("/users/{user_id}/hosts").Version(2).Get().Wrap(checkUser).RouteHandler(makeFetchHosts(sc))
	app.AddRoute("/users/{user_id}/patches").Version(2).Get().Wrap(checkUser).RouteHandler(makeUserPatchHandler(sc))
	app.AddRoute("/users/offboard_user").Version(2).Post().Wrap(checkUser, editRoles).RouteHandler(makeOffboardUser(sc, env))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Get().Wrap(checkUser).RouteHandler(makeGetUserPermissions(sc, evergreen.GetEnvironment().RoleManager()))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Post().Wrap(checkUser, editRoles).RouteHandler(makeModifyUserPermissions(sc, evergreen.GetEnvironment().RoleManager()))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Delete().Wrap(checkUser, editRoles).RouteHandler(makeDeleteUserPermissions(sc, evergreen.GetEnvironment().RoleManager()))
	app.AddRoute("/users/{user_id}/roles").Version(2).Post().Wrap(checkUser, editRoles).RouteHandler(makeModifyUserRoles(sc, evergreen.GetEnvironment().RoleManager()))
	app.AddRoute("/versions").Version(2).Put().Wrap(checkUser).RouteHandler(makeVersionCreateHandler(sc))
	app.AddRoute("/versions/{version_id}").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetVersionByID(sc))
	app.AddRoute("/versions/{version_id}/abort").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeAbortVersion(sc))
	app.AddRoute("/versions/{version_id}/builds").Version(2).Get().Wrap(viewTasks).RouteHandler(makeGetVersionBuilds(sc))
	app.AddRoute("/versions/{version_id}/restart").Version(2).Post().Wrap(checkUser, editTasks).RouteHandler(makeRestartVersion(sc))
	app.AddRoute("/versions/{version_id}/annotations").Version(2).Get().Wrap(checkUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByVersion(sc))

	// Add an options method to every POST request to handle pre-flight Options requests.
	// These requests must not check for credentials and just validate whether a route exists
	// And allows requests from a origin.
	for _, route := range app.Routes() {
		if route.HasMethod("POST") {
			app.AddRoute(route.GetRoute()).Version(2).Options().RouteHandler(makeOptionsHandler())
		}
	}
}
