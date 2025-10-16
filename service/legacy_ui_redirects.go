package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/gimlet"
)

func (uis *UIServer) legacyWaterfallPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject(r.Context())

	if err != nil || project == nil {
		http.Redirect(w, r, uis.Settings.Ui.UIv2Url, http.StatusMovedPermanently)
		return
	}

	newUIURL := fmt.Sprintf("%s/project/%s/waterfall", uis.Settings.Ui.UIv2Url, project.Identifier)
	http.Redirect(w, r, newUIURL, http.StatusMovedPermanently)
}

func (uis *UIServer) legacyDistrosPage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/distros", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyTaskQueue(w http.ResponseWriter, r *http.Request) {
	distro := gimlet.GetVars(r)["distro"]
	taskId := gimlet.GetVars(r)["task_id"]
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/task-queue/%s/%s", uis.Settings.Ui.UIv2Url, distro, taskId)
	}

	http.Redirect(w, r, newUILink, http.StatusTemporaryRedirect)
}

func (uis *UIServer) legacyTaskHistoryPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	executionStr := gimlet.GetVars(r)["execution"]
	var execution int
	var err error

	if executionStr != "" {
		execution, err = strconv.Atoi(executionStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Bad execution number: %v", executionStr), http.StatusBadRequest)
			return
		}
	}
	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("%s/task/%s/history?execution=%s", uis.Settings.Ui.UIv2Url, projCtx.Task.Id, execution), http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyTaskPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	executionStr := gimlet.GetVars(r)["execution"]
	var execution int
	var err error

	if executionStr != "" {
		execution, err = strconv.Atoi(executionStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Bad execution number: %v", executionStr), http.StatusBadRequest)
			return
		}
	}
	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	spruceLink := fmt.Sprintf("%s/task/%s?execution=%d", uis.Settings.Ui.UIv2Url, projCtx.Task.Id, execution)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyProjectsPage(w http.ResponseWriter, r *http.Request) {
	projectId := gimlet.GetVars(r)["project_id"]
	newUIProjectsLink := fmt.Sprintf("%s/project/%s/settings", uis.Settings.Ui.UIv2Url, projectId)
	http.Redirect(w, r, newUIProjectsLink, http.StatusPermanentRedirect)
}
