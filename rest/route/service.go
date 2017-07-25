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
		"/builds/{build_id}":                                   getBuildGetRouteManager,
		"/builds/{build_id}/abort":                             getBuildAbortRouteManager,
		"/builds/{build_id}/tasks":                             getTasksByBuildRouteManager,
		"/distros":                                             getDistroRouteManager,
		"/hosts":                                               getHostRouteManager,
		"/hosts/{host_id}":                                     getHostIDRouteManager,
		"/patches/{patch_id}":                                  getPatchByIdManager,
		"/patches/{patch_id}/abort":                            getPatchAbortManager,
		"/patches/{patch_id}/restart":                          getPatchRestartManager,
		"/projects":                                            getProjectRouteManager,
		"/projects/{project_id}/patches":                       getPatchesByProjectManager,
		"/projects/{project_id}/revisions/{commit_hash}/tasks": getTasksByProjectAndCommitRouteManager,
		"/tasks/{task_id}":                                     getTaskRouteManager,
		"/tasks/{task_id}/metrics/process":                     getTaskProcessMetricsManager,
		"/tasks/{task_id}/metrics/system":                      getTaskSystemMetricsManager,
		"/tasks/{task_id}/restart":                             getTaskRestartRouteManager,
		"/tasks/{task_id}/tests":                               getTestRouteManager,
		"/cost/version/{version_id}":                           getCostByVersionIdRouteManager,
		"/cost/distro/{distro_id}":                             getCostByDistroIdRouteManager,
		"/versions/{version_id}":                               getVersionIdRouteManager,
		"/versions/{version_id}/builds":                        getBuildsForVersionRouteManager,
		"/versions/{version_id}/abort":                         getAbortVersionRouteManager,
		"/versions/{version_id}/restart":                       getRestartVersionRouteManager,
	}

	for path, getManager := range routes {
		getManager(path, 2).Register(r, sc)
	}

	return r
}
