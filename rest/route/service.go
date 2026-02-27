package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/acl"
	"github.com/gorilla/handlers"
	"github.com/mongodb/amboy"
)

const defaultLimit = 100

type HandlerOpts struct {
	APIQueue            amboy.Queue
	TaskDispatcher      model.TaskQueueItemDispatcher
	TaskAliasDispatcher model.TaskQueueItemDispatcher
	URL                 string
	GithubSecret        []byte
}

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(app *gimlet.APIApp, opts HandlerOpts) {
	sc := &data.DBConnector{}
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	parsleyURL := settings.Ui.ParsleyUrl

	sc.SetURL(opts.URL)

	// Middleware
	requireUser := gimlet.NewRequireAuthHandler()
	requireUserOrTask := NewUserOrTaskAuthMiddleware()
	requireValidGithubPayload := NewGithubAuthMiddleware()
	requireValidSNSPayload := NewSNSAuthMiddleware()
	requireTask := NewTaskAuthMiddleware()
	requireHost := NewHostAuthMiddleware()
	addProject := NewProjectContextMiddleware()
	requireProjectAdmin := NewProjectAdminMiddleware()
	requireAlertmanager := NewAlertmanagerMiddleware()
	requireBackstage := newBackstageMiddleware()
	createProject := NewCanCreateMiddleware()
	adminSettings := RequiresSuperUserPermission(evergreen.PermissionAdminSettings, evergreen.AdminSettingsEdit)
	createDistro := RequiresSuperUserPermission(evergreen.PermissionDistroCreate, evergreen.DistroCreate)
	editRoles := RequiresSuperUserPermission(evergreen.PermissionRoleModify, evergreen.RoleModify)
	viewTasks := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	editTasks := RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksBasic)
	editAnnotations := RequiresProjectPermission(evergreen.PermissionAnnotations, evergreen.AnnotationsModify)
	viewAnnotations := RequiresProjectPermission(evergreen.PermissionAnnotations, evergreen.AnnotationsView)
	submitPatches := RequiresProjectPermission(evergreen.PermissionPatches, evergreen.PatchSubmit)
	viewProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsView)
	editProjectSettings := RequiresProjectPermission(evergreen.PermissionProjectSettings, evergreen.ProjectSettingsEdit)
	viewDistroSettings := RequiresDistroPermission(evergreen.PermissionDistroSettings, evergreen.DistroSettingsView)
	editDistroSettings := RequiresDistroPermission(evergreen.PermissionDistroSettings, evergreen.DistroSettingsEdit)
	removeDistroSettings := RequiresDistroPermission(evergreen.PermissionDistroSettings, evergreen.DistroSettingsAdmin)
	compress := gimlet.WrapperHandlerMiddleware(handlers.CompressHandler)

	app.AddWrapper(gimlet.WrapperMiddleware(allowCORS))

	// Clients
	stsManager := cloud.GetSTSManager(false)

	// Agent protocol routes
	app.AddRoute("/agent/perf_monitoring_url").Version(2).Get().Wrap(requireHost).RouteHandler(makeGetPerfURL(settings.PerfMonitoringURL))
	app.AddRoute("/agent/setup").Version(2).Get().Wrap(requireHost).RouteHandler(makeAgentSetup(settings))
	app.AddRoute("/distros/{distro_id}/ami").Version(2).Get().Wrap(requireTask).RouteHandler(makeGetDistroAMI())
	app.AddRoute("/distros/{distro_id}/client_urls").Version(2).Get().Wrap(requireHost).RouteHandler(makeGetDistroClientURLs(env))
	app.AddRoute("/hosts/{host_id}/agent/next_task").Version(2).Get().Wrap(requireHost).RouteHandler(makeHostAgentNextTask(env, opts.TaskDispatcher, opts.TaskAliasDispatcher))
	app.AddRoute("/hosts/{host_id}/task/{task_id}/end").Version(2).Post().Wrap(requireHost, requireTask).RouteHandler(makeHostAgentEndTask(env))
	app.AddRoute("/hosts/{host_id}/disable").Version(2).Post().Wrap(requireHost).RouteHandler(makeDisableHostHandler(env))
	app.AddRoute("/hosts/{host_id}/is_up").Version(2).Post().Wrap(requireHost).RouteHandler(makeHostIsUpPostHandler(env))
	app.AddRoute("/hosts/{host_id}/provisioning_options").Version(2).Get().Wrap(requireHost).RouteHandler(makeHostProvisioningOptionsGetHandler(env))
	app.AddRoute("/hosts/{task_id}/create").Version(2).Post().Wrap(requireTask).RouteHandler(makeHostCreateRouteManager(env))
	app.AddRoute("/hosts/{task_id}/list").Version(2).Get().Wrap(requireTask).RouteHandler(makeHostListRouteManager())
	app.AddRoute("/task/{task_id}/").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeFetchTask())
	app.AddRoute("/task/{task_id}/display_task").Version(2).Get().Wrap(requireTask).RouteHandler(makeGetDisplayTaskHandler())
	app.AddRoute("/task/{task_id}/distro_view").Version(2).Get().Wrap(requireTask, requireHost).RouteHandler(makeGetDistroView())
	app.AddRoute("/task/{task_id}/host_view").Version(2).Get().Wrap(requireTask, requireHost).RouteHandler(makeGetHostView())
	app.AddRoute("/task/{task_id}/downstreamParams").Version(2).Post().Wrap(requireTask).RouteHandler(makeSetDownstreamParams())
	app.AddRoute("/task/{task_id}/expansions_and_vars").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeGetExpansionsAndVars(settings))
	app.AddRoute("/task/{task_id}/files").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeAttachFiles())
	app.AddRoute("/task/{task_id}/generate").Version(2).Post().Wrap(requireTask).RouteHandler(makeGenerateTasksHandler(env))
	app.AddRoute("/task/{task_id}/generate").Version(2).Get().Wrap(requireTask).RouteHandler(makeGenerateTasksPollHandler())
	app.AddRoute("/task/{task_id}/new_push").Version(2).Post().Wrap(requireTask).RouteHandler(makeNewPush())
	app.AddRoute("/task/{task_id}/heartbeat").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeHeartbeat())
	app.AddRoute("/task/{task_id}/parser_project").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeGetParserProject(env))
	app.AddRoute("/task/{task_id}/project_ref").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeGetProjectRef())
	app.AddRoute("/task/{task_id}/set_results_info").Version(2).Post().Wrap(requireTask).RouteHandler(makeSetTaskResultsInfoHandler())
	app.AddRoute("/task/{task_id}/start").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeStartTask(env))
	app.AddRoute("/task/{task_id}/test_logs").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeAttachTestLog(settings))
	app.AddRoute("/task/{task_id}/test_results").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeAttachTestResults(env))
	app.AddRoute("/task/{task_id}/patch").Version(2).Get().Wrap(requireTask).RouteHandler(makeServePatch())
	app.AddRoute("/task/{task_id}/version").Version(2).Get().Wrap(requireTask).RouteHandler(makeServeVersion())
	app.AddRoute("/task/{task_id}/git/patchfile/{patchfile_id}").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeGitServePatchFile())
	app.AddRoute("/task/{task_id}/github_dynamic_access_token").Version(2).Delete().Wrap(requireTask).RouteHandler(makeRevokeGitHubDynamicAccessToken(env))
	app.AddRoute("/task/{task_id}/installation_token/{owner}/{repo}").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeCreateInstallationToken(env))
	app.AddRoute("/task/{task_id}/github_dynamic_access_token/{owner}/{repo}").Version(2).Post().Wrap(requireTask).RouteHandler(makeCreateGitHubDynamicAccessToken(env))
	app.AddRoute("/task/{task_id}/keyval/inc").Version(2).Post().Wrap(requireTask).RouteHandler(makeKeyvalPluginInc())
	app.AddRoute("/task/{task_id}/manifest/load").Version(2).Get().Wrap(requireUserOrTask).RouteHandler(makeManifestLoad(settings))
	app.AddRoute("/task/{task_id}/update_push_status").Version(2).Post().Wrap(requireTask).RouteHandler(makeUpdatePushStatus())
	app.AddRoute("/task/{task_id}/restart").Version(2).Post().Wrap(requireTask).RouteHandler(makeMarkTaskForRestart())
	app.AddRoute("/task/{task_id}/check_run").Version(2).Post().Wrap(requireTask).RouteHandler(makeCheckRun(settings))
	app.AddRoute("/task/{task_id}/aws/assume_role").Version(2).Post().Wrap(requireTask, requireHost).RouteHandler(makeAWSAssumeRole(stsManager))

	// REST v2 API Routes
	app.AddRoute("/").Version(2).Get().Wrap(requireUser).RouteHandler(makePlaceHolder())
	app.AddRoute("/admin/banner").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchAdminBanner())
	app.AddRoute("/admin/banner").Version(2).Post().Wrap(requireUser, adminSettings).RouteHandler(makeSetAdminBanner())
	app.AddRoute("/admin/uiv2_url").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchAdminUIV2Url())
	app.AddRoute("/admin/events").Version(2).Get().Wrap(requireUser, adminSettings).RouteHandler(makeFetchAdminEvents())
	app.AddRoute("/admin/spawn_hosts").Version(2).Get().Wrap(requireUser, adminSettings).RouteHandler(makeFetchSpawnHostUsage())
	app.AddRoute("/admin/restart/tasks").Version(2).Post().Wrap(adminSettings).RouteHandler(makeRestartRoute(opts.APIQueue))
	app.AddRoute("/admin/revert").Version(2).Post().Wrap(requireUser, adminSettings).RouteHandler(makeRevertRouteManager())
	app.AddRoute("/admin/service_flags").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchServiceFlags())
	app.AddRoute("/admin/service_flags").Version(2).Post().Wrap(requireUser, adminSettings).RouteHandler(makeSetServiceFlagsRouteManager())
	app.AddRoute("/admin/settings").Version(2).Get().Wrap(requireUser, adminSettings).RouteHandler(makeFetchAdminSettings())
	app.AddRoute("/admin/settings").Version(2).Post().Wrap(requireUser, adminSettings).RouteHandler(makeSetAdminSettings())
	app.AddRoute("/admin/task_limits").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchTaskLimits())
	app.AddRoute("/admin/task_queue").Version(2).Delete().Wrap(requireUser, adminSettings).RouteHandler(makeClearTaskQueueHandler())
	app.AddRoute("/admin/service_users").Version(2).Get().Wrap(requireUser, adminSettings).RouteHandler(makeGetServiceUsers())
	app.AddRoute("/admin/service_users").Version(2).Post().Wrap(requireUser, adminSettings).RouteHandler(makeUpdateServiceUser())
	app.AddRoute("/admin/service_users").Version(2).Delete().Wrap(requireUser, adminSettings).RouteHandler(makeDeleteServiceUser())
	app.AddRoute("/alias/{project_id}").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchAliases())
	app.AddRoute("/auth").Version(2).Get().Wrap(requireUser).RouteHandler(&authPermissionGetHandler{})
	app.AddRoute("/builds/{build_id}").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetBuildByID(env))
	app.AddRoute("/builds/{build_id}").Version(2).Patch().Wrap(requireUser, editTasks).RouteHandler(makeChangeStatusForBuild())
	app.AddRoute("/builds/{build_id}/abort").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeAbortBuild())
	app.AddRoute("/builds/{build_id}/restart").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeRestartBuild())
	app.AddRoute("/builds/{build_id}/tasks").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeFetchTasksByBuild(parsleyURL))
	app.AddRoute("/builds/{build_id}/annotations").Version(2).Get().Wrap(requireUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByBuild())
	// degraded_mode is used by Kanopy's alertmanager instance, which is only able to perform basic auth for REST, so it does not pass in user info
	app.AddRoute("/degraded_mode").Version(2).Post().Wrap(requireAlertmanager).RouteHandler(makeSetDegradedMode())
	// Do not apply viewDistroSettings middleware, as it requires a specific distro ID.
	app.AddRoute("/distros").Version(2).Get().Wrap(requireUser).RouteHandler(makeDistroRoute())
	app.AddRoute("/distros/{distro_id}").Version(2).Get().Wrap(requireUser, viewDistroSettings).RouteHandler(makeGetDistroByID())
	app.AddRoute("/distros/{distro_id}").Version(2).Patch().Wrap(requireUser, editDistroSettings).RouteHandler(makePatchDistroByID())
	app.AddRoute("/distros/{distro_id}").Version(2).Delete().Wrap(requireUser, removeDistroSettings).RouteHandler(makeDeleteDistroByID())
	app.AddRoute("/distros/{distro_id}").Version(2).Put().Wrap(requireUser, createDistro).RouteHandler(makePutDistro())
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Get().Wrap(requireUser, editDistroSettings).RouteHandler(makeGetDistroSetup())
	app.AddRoute("/distros/{distro_id}/setup").Version(2).Patch().Wrap(requireUser, editDistroSettings).RouteHandler(makeChangeDistroSetup())
	app.AddRoute("/distros/{distro_id}/copy/{new_distro_id}").Version(2).Put().Wrap(requireUser, editDistroSettings).RouteHandler(makeCopyDistro())

	app.AddRoute("/hooks/github").Version(2).Post().Wrap(requireValidGithubPayload).RouteHandler(makeGithubHooksRoute(sc, opts.APIQueue, opts.GithubSecret, settings))
	app.AddRoute("/hooks/aws").Version(2).Post().Wrap(requireValidSNSPayload).RouteHandler(makeEC2SNS(env, opts.APIQueue))

	app.AddRoute("/host/filter").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchHostFilter())
	app.AddRoute("/host/start_processes").Version(2).Post().Wrap(requireUser).RouteHandler(makeHostStartProcesses(env))
	app.AddRoute("/host/get_processes").Version(2).Get().Wrap(requireUser).RouteHandler(makeHostGetProcesses(env))
	app.AddRoute("/hosts").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchHosts())
	app.AddRoute("/hosts").Version(2).Post().Wrap(requireUser).RouteHandler(makeSpawnHostCreateRoute(env))
	app.AddRoute("/hosts").Version(2).Patch().Wrap(requireUser).RouteHandler(makeChangeHostsStatuses(env))
	app.AddRoute("/hosts/{host_id}").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetHostByID())
	app.AddRoute("/hosts/{host_id}").Version(2).Patch().Wrap(requireUser).RouteHandler(makeHostModifyRouteManager(env))
	app.AddRoute("/hosts/{host_id}/stop").Version(2).Post().Wrap(requireUser).RouteHandler(makeHostStopManager(env))
	app.AddRoute("/hosts/{host_id}/start").Version(2).Post().Wrap(requireUser).RouteHandler(makeHostStartManager(env))
	app.AddRoute("/hosts/{host_id}/change_password").Version(2).Post().Wrap(requireUser).RouteHandler(makeHostChangePassword(env))
	app.AddRoute("/hosts/{host_id}/extend_expiration").Version(2).Post().Wrap(requireUser).RouteHandler(makeExtendHostExpiration())
	app.AddRoute("/hosts/{host_id}/terminate").Version(2).Post().Wrap(requireUser).RouteHandler(makeTerminateHostRoute())
	app.AddRoute("/hosts/{host_id}/attach").Version(2).Post().Wrap(requireUser).RouteHandler(makeAttachVolume(env))
	app.AddRoute("/hosts/{host_id}/detach").Version(2).Post().Wrap(requireUser).RouteHandler(makeDetachVolume(env))
	app.AddRoute("/hosts/ip_address/{ip_address}").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetHostByIpAddress())
	app.AddRoute("/volumes").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetVolumes())
	app.AddRoute("/volumes").Version(2).Post().Wrap(requireUser).RouteHandler(makeCreateVolume(env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Wrap(requireUser).Delete().RouteHandler(makeDeleteVolume(env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Wrap(requireUser).Patch().RouteHandler(makeModifyVolume(env))
	app.AddRoute("/volumes/{volume_id}").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetVolumeByID())
	app.AddRoute("/keys").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchKeys())
	app.AddRoute("/keys").Version(2).Post().Wrap(requireUser).RouteHandler(makeSetKey())
	app.AddRoute("/keys/{key_name}").Version(2).Delete().Wrap(requireUser).RouteHandler(makeDeleteKeys())
	app.AddRoute("/notifications/{type}").Version(2).Post().Wrap(requireUser).RouteHandler(makeNotification(env))
	app.AddRoute("/panic").Version(2).Post().Wrap(requireUser).RouteHandler(makePanicReport())
	app.AddRoute("/patches/{patch_id}").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeFetchPatchByID())
	app.AddRoute("/patches/{patch_id}").Version(2).Patch().Wrap(requireUser, submitPatches).RouteHandler(makeChangePatchStatus(env))
	app.AddRoute("/patches/{patch_id}/abort").Version(2).Post().Wrap(requireUser, submitPatches).RouteHandler(makeAbortPatch())
	app.AddRoute("/patches/{patch_id}/configure").Version(2).Post().Wrap(requireUser, submitPatches).RouteHandler(makeSchedulePatchHandler(env))
	app.AddRoute("/patches/{patch_id}/raw").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makePatchRawHandler())
	app.AddRoute("/patches/{patch_id}/raw_modules").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeModuleRawHandler())
	app.AddRoute("/patches/{patch_id}/restart").Version(2).Post().Wrap(requireUser, submitPatches).RouteHandler(makeRestartPatch())
	app.AddRoute("/patches/{patch_id}/estimated_generated_tasks").Version(2).Get().Wrap(requireUser).RouteHandler(makeCountEstimatedGeneratedTasks())
	app.AddRoute("/projects").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchProjectsRoute())
	app.AddRoute("/projects/test_alias").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetProjectAliasResultsHandler())
	app.AddRoute("/projects/{project_id}").Version(2).Delete().Wrap(requireUser, addProject, requireProjectAdmin, editProjectSettings).RouteHandler(makeDeleteProject())
	app.AddRoute("/projects/{project_id}").Version(2).Get().Wrap(requireUser, addProject, viewProjectSettings).RouteHandler(makeGetProjectByID())
	app.AddRoute("/projects/{project_id}").Version(2).Patch().Wrap(requireUser, addProject, requireProjectAdmin, editProjectSettings).RouteHandler(makePatchProjectByID(settings))
	app.AddRoute("/projects/{project_id}/repotracker").Version(2).Post().Wrap(requireUser, addProject).RouteHandler(makeRunRepotrackerForProject())
	app.AddRoute("/projects/{project_id}").Version(2).Put().Wrap(requireUser, createProject).RouteHandler(makePutProjectByID(env))
	app.AddRoute("/projects/{project_id}/copy").Version(2).Post().Wrap(requireUser, addProject, requireProjectAdmin, editProjectSettings).RouteHandler(makeCopyProject(env))
	app.AddRoute("/projects/{project_id}/copy/variables").Version(2).Post().Wrap(requireUser, addProject, requireProjectAdmin, editProjectSettings).RouteHandler(makeCopyVariables())
	app.AddRoute("/projects/{project_id}/backstage_variables").Version(2).Post().Wrap(requireUser, requireBackstage).RouteHandler(makeBackstageVariablesPost())
	app.AddRoute("/projects/{project_id}/events").Version(2).Get().Wrap(requireUser, addProject, requireProjectAdmin, viewProjectSettings).RouteHandler(makeFetchProjectEvents())
	app.AddRoute("/projects/{project_id}/patches").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makePatchesByProjectRoute())
	app.AddRoute("/projects/{project_id}/recent_versions").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeFetchProjectVersionsLegacy())
	app.AddRoute("/projects/{project_id}/revisions/{commit_hash}/tasks").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeTasksByProjectAndCommitHandler(parsleyURL))
	app.AddRoute("/projects/{project_id}/task_reliability").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetProjectTaskReliability())
	app.AddRoute("/projects/{project_id}/task_stats").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetProjectTaskStats())
	app.AddRoute("/projects/{project_id}/versions").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetProjectVersionsHandler())
	app.AddRoute("/projects/{project_id}/versions").Version(2).Patch().Wrap(requireUser, requireProjectAdmin).RouteHandler(makeModifyProjectVersionsHandler(opts.URL))
	app.AddRoute("/projects/{project_id}/tasks/{task_name}").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetProjectTasksHandler(opts.URL))
	app.AddRoute("/projects/{project_id}/task_executions").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetProjectTaskExecutionsHandler())
	app.AddRoute("/projects/{project_id}/patch_trigger_aliases").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeFetchPatchTriggerAliases())
	app.AddRoute("/projects/{project_id}/parameters").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeFetchParameters())
	app.AddRoute("/permissions").Version(2).Get().Wrap(requireUser).RouteHandler(&permissionsGetHandler{})
	app.AddRoute("/permissions/users").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetAllUsersPermissions(env.RoleManager()))
	app.AddRoute("/roles").Version(2).Get().Wrap(requireUser).RouteHandler(acl.NewGetAllRolesHandler(env.RoleManager()))
	app.AddRoute("/roles/{role_id}/users").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetUsersWithRole())
	app.AddRoute("/select/tests").Version(2).Post().Wrap(requireUserOrTask).RouteHandler(makeSelectTestsHandler(env))
	app.AddRoute("/status/cli_version").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchCLIVersionRoute(env))
	app.AddRoute("/status/hosts/distros").Version(2).Get().Wrap(requireUser).RouteHandler(makeHostStatusByDistroRoute())
	app.AddRoute("/status/notifications").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchNotifcationStatusRoute())
	app.AddRoute("/status/recent_tasks").Version(2).Get().Wrap(requireUser).RouteHandler(makeRecentTaskStatusHandler())
	app.AddRoute("/subscriptions").Version(2).Delete().Wrap(requireUser).RouteHandler(makeDeleteSubscription())
	app.AddRoute("/subscriptions").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchSubscription())
	app.AddRoute("/subscriptions").Version(2).Post().Wrap(requireUser).RouteHandler(makeSetSubscription())
	app.AddRoute("/tasks/{task_id}").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetTaskRoute(parsleyURL, opts.URL))
	app.AddRoute("/tasks/{task_id}").Version(2).Patch().Wrap(requireUser, addProject, editTasks).RouteHandler(makeModifyTaskRoute())
	app.AddRoute("/tasks/{task_id}/artifacts/url").Version(2).Patch().Wrap(requireUser, addProject, requireProjectAdmin, editTasks).RouteHandler(makeUpdateArtifactURLRoute())
	app.AddRoute("/tasks/{task_id}/annotations").Version(2).Get().Wrap(requireUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByTask())
	app.AddRoute("/tasks/{task_id}/annotation").Version(2).Put().Wrap(requireUser, editAnnotations).RouteHandler(makePutAnnotationsByTask())
	app.AddRoute("/tasks/{task_id}/annotation").Version(2).Patch().Wrap(requireUser, editAnnotations).RouteHandler(makePatchAnnotationsByTask())
	app.AddRoute("/tasks/{task_id}/created_ticket").Version(2).Put().Wrap(requireUser, editAnnotations).RouteHandler(makeCreatedTicketByTask())
	app.AddRoute("/tasks/{task_id}/abort").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeTaskAbortHandler())
	app.AddRoute("/tasks/{task_id}/manifest").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetManifestHandler())
	app.AddRoute("/tasks/{task_id}/restart").Version(2).Post().Wrap(requireUser, addProject, editTasks).RouteHandler(makeTaskRestartHandler())
	app.AddRoute("/tasks/{task_id}/tests").Version(2).Get().Wrap(requireUser, addProject, viewTasks).RouteHandler(makeFetchTestsForTask(env, sc))
	app.AddRoute("/tasks/{task_id}/tests/count").Version(2).Get().Wrap(requireUser, addProject, viewTasks).RouteHandler(makeFetchTestCountForTask())
	app.AddRoute("/tasks/{task_id}/generated_tasks").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetGeneratedTasks())
	app.AddRoute("/tasks/{task_id}/build/TaskLogs").Version(2).Get().Wrap(requireUser, viewTasks, compress).RouteHandler(makeGetTaskLogs(opts.URL))
	app.AddRoute("/tasks/{task_id}/build/TestLogs/{path}").Version(2).Get().Wrap(requireUser, viewTasks, compress).RouteHandler(makeGetTestLogs(opts.URL))
	app.AddRoute("/tasks/{task_id}/github_dynamic_access_tokens").Version(2).Delete().Wrap(requireUser, viewTasks).RouteHandler(makeDeleteGitHubDynamicAccessTokens())
	app.AddRoute("/user/settings").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchUserConfig())
	app.AddRoute("/user/settings").Version(2).Post().Wrap(requireUser).RouteHandler(makeSetUserConfig())
	app.AddRoute("/users/{user_id}").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetUserHandler())
	app.AddRoute("/users/{user_id}/hosts").Version(2).Get().Wrap(requireUser).RouteHandler(makeFetchHosts())
	app.AddRoute("/users/{user_id}/patches").Version(2).Get().Wrap(requireUser).RouteHandler(makeUserPatchHandler())
	app.AddRoute("/users/offboard_user").Version(2).Post().Wrap(requireUser, editRoles).RouteHandler(makeOffboardUser(env))
	app.AddRoute("/users/rename_user").Version(2).Post().Wrap(requireUser, editRoles).RouteHandler(makeRenameUser(env))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Get().Wrap(requireUser).RouteHandler(makeGetUserPermissions(env.RoleManager()))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Post().Wrap(requireUser, editRoles).RouteHandler(makeModifyUserPermissions(env.RoleManager()))
	app.AddRoute("/users/{user_id}/permissions").Version(2).Delete().Wrap(requireUser, editRoles).RouteHandler(makeDeleteUserPermissions(env.RoleManager()))
	app.AddRoute("/users/{user_id}/roles").Version(2).Post().Wrap(requireUser, editRoles).RouteHandler(makeModifyUserRoles(env.RoleManager()))
	app.AddRoute("/validate").Version(2).Post().Wrap(requireUser).RouteHandler(makeValidateProject())
	app.AddRoute("/versions/{version_id}").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetVersionByID())
	app.AddRoute("/versions/{version_id}").Version(2).Patch().Wrap(requireUser, editTasks).RouteHandler(makePatchVersion())
	app.AddRoute("/versions/{version_id}/abort").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeAbortVersion())
	app.AddRoute("/versions/{version_id}/activate_tasks").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeActivateVersionTasks())
	app.AddRoute("/versions/{version_id}/builds").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetVersionBuilds(env))
	app.AddRoute("/versions/{version_id}/restart").Version(2).Post().Wrap(requireUser, editTasks).RouteHandler(makeRestartVersion())
	app.AddRoute("/versions/{version_id}/annotations").Version(2).Get().Wrap(requireUser, viewAnnotations).RouteHandler(makeFetchAnnotationsByVersion())
	app.AddRoute("/versions/{version_id}/manifest").Version(2).Get().Wrap(requireUser, viewTasks).RouteHandler(makeGetVersionManifest())

	// Add an options method to every GET, POST request to handle pre-flight Options requests.
	// These requests must not check for credentials and just validate whether a route exists
	// And allows requests from a origin.
	for _, route := range app.Routes() {
		if route.HasMethod(http.MethodPost) || route.HasMethod(http.MethodGet) {
			app.AddRoute(route.GetRoute()).Version(2).Options().RouteHandler(makeOptionsHandler())
		}
	}
}
