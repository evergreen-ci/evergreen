package rest

import (
	"10gen.com/mci/web"
	"github.com/gorilla/mux"
)

func RegisterRoutes(ae web.HandlerApp, rootRouter *mux.Router) {
	router := rootRouter.PathPrefix("/rest/v1/").Subrouter().StrictSlash(true)

	router.HandleFunc("/projects/{project_name}/versions",
		ae.MakeHandler(getRecentVersions)).Name("recent_versions").Methods("GET")

	router.HandleFunc("/projects/{project_name}/revisions/{revision}",
		ae.MakeHandler(getVersionInfoViaRevision)).Name("version_info_via_revision").Methods("GET")

	router.HandleFunc("/versions/{version_id}",
		ae.MakeHandler(getVersionInfo)).Name("version_info").Methods("GET")

	router.HandleFunc("/versions/{version_id}",
		ae.MakeHandler(modifyVersionInfo, getVersionInfo)).Methods("PATCH")

	router.HandleFunc("/versions/{version_id}/status",
		ae.MakeHandler(getVersionStatus)).Name("version_status").Methods("GET")

	router.HandleFunc("/builds/{build_id}",
		ae.MakeHandler(getBuildInfo)).Name("build_info").Methods("GET")

	router.HandleFunc("/builds/{build_id}/status",
		ae.MakeHandler(getBuildStatus)).Name("build_status").Methods("GET")

	router.HandleFunc("/tasks/{task_id}",
		ae.MakeHandler(getTaskInfo)).Name("task_info").Methods("GET")

	router.HandleFunc("/tasks/{task_id}/status",
		ae.MakeHandler(getTaskStatus)).Name("task_status").Methods("GET")

	router.HandleFunc("/tasks/{task_name}/history",
		ae.MakeHandler(getTaskHistory)).Name("task_history").Methods("GET")
}
