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
		"/builds/{build_id}/tasks": getTasksByBuildRouteManager,
		"/hosts":                   getHostRouteManager,
		"/projects/{project_id}/revisions/{commit_hash}/tasks": getTasksByProjectAndCommitRouteManager,
		"/tasks/{task_id}":                                     getTaskRouteManager,
		"/tasks/{task_id}/restart":                             getTaskRestartRouteManager,
		"/tasks/{task_id}/tests":                               getTestRouteManager,
		"/": getPlaceHolderManger,
	}

	for path, getManager := range routes {
		getManager(path, 2).Register(r, sc)
	}

	return r
}
