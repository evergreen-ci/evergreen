package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(root *mux.Router, queue amboy.Queue, URL, prefix string, superUsers []string, githubSecret []byte) http.Handler {
	sc := &data.DBConnector{}

	sc.SetURL(URL)
	sc.SetPrefix(prefix)
	sc.SetSuperUsers(superUsers)
	return GetHandler(root, sc, queue, githubSecret)
}

// GetHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func GetHandler(r *mux.Router, sc data.Connector, queue amboy.Queue, githubSecret []byte) http.Handler {
	routes := map[string]routeManagerFactory{
		"/":                                  getPlaceHolderManger,
		"/admin":                             getLegacyAdminSettingsManager,
		"/admin/banner":                      getBannerRouteManager,
		"/admin/restart":                     getRestartRouteManager(queue),
		"/admin/revert":                      getRevertRouteManager,
		"/admin/service_flags":               getServiceFlagsRouteManager,
		"/admin/settings":                    getAdminSettingsManager,
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
		"/projects/{project_id}/revisions/{commit_hash}/tasks": getTasksByProjectAndCommitRouteManager,
		"/status/cli_version":                                  getCLIVersionRouteManager,
		"/status/hosts/distros":                                getHostStatsByDistroManager,
		"/status/recent_tasks":                                 getRecentTasksRouteManager,
		"/tasks/{task_id}":                                     getTaskRouteManager,
		"/tasks/{task_id}/abort":                               getTaskAbortManager,
		"/tasks/{task_id}/generate":                            getGenerateManager,
		"/tasks/{task_id}/metrics/process":                     getTaskProcessMetricsManager,
		"/tasks/{task_id}/metrics/system":                      getTaskSystemMetricsManager,
		"/tasks/{task_id}/restart":                             getTaskRestartRouteManager,
		"/tasks/{task_id}/tests":                               getTestRouteManager,
		"/users/{user_id}/hosts":                               getHostsByUserManager,
		"/users/{user_id}/patches":                             getPatchesByUserManager,
		"/versions/{version_id}":                               getVersionIdRouteManager,
		"/versions/{version_id}/abort":                         getAbortVersionRouteManager,
		"/versions/{version_id}/builds":                        getBuildsForVersionRouteManager,
		"/versions/{version_id}/restart":                       getRestartVersionRouteManager,
	}

	for path, getManager := range routes {
		getManager(path, 2).Register(r, sc)
	}

	return r
}
