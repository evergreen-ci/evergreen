package rest

import (
	"10gen.com/mci"
	"net/http"
)

type RouteInfo struct {
	Path    string
	Handler http.HandlerFunc
	Name    string
	Method  string
}

type restUISAPI interface {
	WriteJSON(w http.ResponseWriter, status int, data interface{})
	GetMCISettings() mci.MCISettings
}

type restAPI struct {
	restUISAPI
}

func GetRestRoutes(ruis restUISAPI) []RouteInfo {
	restapi := restAPI{ruis}
	return []RouteInfo{
		{"/projects/{project_id}/versions", restapi.getRecentVersions, "recent_versions", "GET"},
		{"/projects/{project_id}/revisions/{revision}", restapi.getVersionInfoViaRevision, "version_info_via_revision", "GET"},
		{"/versions/{version_id}", restapi.getVersionInfo, "version_info", "GET"},
		{"/versions/{version_id}", restapi.modifyVersionInfo, "", "PATCH"},
		{"/versions/{version_id}/status", restapi.getVersionStatus, "version_status", "GET"},
		{"/builds/{build_id}", restapi.getBuildInfo, "build_info", "GET"},
		{"/builds/{build_id}/status", restapi.getBuildStatus, "build_status", "GET"},
		{"/tasks/{task_id}", restapi.getTaskInfo, "task_info", "GET"},
		{"/tasks/{task_id}/status", restapi.getTaskStatus, "task_status", "GET"},
		{"/tasks/{task_name}/history", restapi.getTaskHistory, "task_history", "GET"},
	}
}
