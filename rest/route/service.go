package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/gorilla/mux"
)

// AttachHandler attaches the api's request handlers to the given mux router.
// It builds a Connector then attaches each of the main functions for
// the api to the router.
func AttachHandler(root *mux.Router, superUsers []string, URL, prefix string) http.Handler {
	sc := &data.DBConnector{}

	sc.SetURL(URL)
	sc.SetPrefix(prefix)
	sc.SetSuperUsers(superUsers)
	return GetHandler(root, sc)
}

// GetHandler builds each of the functions that this api implements and then
// registers them on the given router. It then returns the given router as an
// http handler which can be given more functions.
func GetHandler(r *mux.Router, sc data.Connector) http.Handler {
	routes := map[string]routeManagerFactory{
		"/":                                                    getPlaceHolderManger,
		"/admin":                                               getAdminSettingsManager,
		"/admin/banner":                                        getBannerRouteManager,
		"/admin/service_flags":                                 getServiceFlagsRouteManager,
		"/admin/restart":                                       getRestartRouteManager,
		"/builds/{build_id}":                                   getBuildByIdRouteManager,
		"/builds/{build_id}/abort":                             getBuildAbortRouteManager,
		"/builds/{build_id}/restart":                           getBuildRestartManager,
		"/builds/{build_id}/tasks":                             getTasksByBuildRouteManager,
		"/distros":                                             getDistroRouteManager,
		"/hosts":                                               getHostRouteManager,
		"/hosts/{host_id}":                                     getHostIDRouteManager,
		"/patches/{patch_id}":                                  getPatchByIdManager,
		"/users/{user_id}/patches":                             getPatchesByUserManager,
		"/users/{user_id}/hosts":                               getHostsByUserManager,
		"/patches/{patch_id}/abort":                            getPatchAbortManager,
		"/patches/{patch_id}/restart":                          getPatchRestartManager,
		"/projects":                                            getProjectRouteManager,
		"/projects/{project_id}/patches":                       getPatchesByProjectManager,
		"/projects/{project_id}/revisions/{commit_hash}/tasks": getTasksByProjectAndCommitRouteManager,
		"/tasks/{task_id}":                                     getTaskRouteManager,
		"/tasks/{task_id}/abort":                               getTaskAbortManager,
		"/tasks/{task_id}/restart":                             getTaskRestartRouteManager,
		"/tasks/{task_id}/tests":                               getTestRouteManager,
		"/tasks/{task_id}/metrics/process":                     getTaskProcessMetricsManager,
		"/tasks/{task_id}/metrics/system":                      getTaskSystemMetricsManager,
		"/cost/version/{version_id}":                           getCostByVersionIdRouteManager,
		"/cost/distro/{distro_id}":                             getCostByDistroIdRouteManager,
		"/cost/project/{project_id}/tasks":                     getCostTaskByProjectRouteManager,
		"/versions/{version_id}":                               getVersionIdRouteManager,
		"/versions/{version_id}/builds":                        getBuildsForVersionRouteManager,
		"/versions/{version_id}/abort":                         getAbortVersionRouteManager,
		"/versions/{version_id}/restart":                       getRestartVersionRouteManager,
		"/status/hosts/distros":                                getHostStatsByDistroManager,
		"/status/recent_tasks":                                 getRecentTasksRouteManager,
		"/keys":                                                getKeysRouteManager,
		"/keys/{key_name}":                                     getKeysDeleteRouteManager,
		"/third_party/github":                                  getGithubHooksRouteManager,
	}

	for path, getManager := range routes {
		getManager(path, 2).Register(r, sc)
	}

	return r
}
