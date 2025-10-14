package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/gimlet"
)

// This file contains legacy route handlers that perform redirects to the new UI (Spruce).
// These functions are kept separate to maintain clean separation between redirect logic
// and actual application functionality.

// legacyWaterfallPage implements a permanent redirect to the new UI waterfall page
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

// legacyDistrosPage redirects to the new UI distros page
func (uis *UIServer) legacyDistrosPage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/distros", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

// legacyTaskQueue redirects to the new UI task queue page
func (uis *UIServer) legacyTaskQueue(w http.ResponseWriter, r *http.Request) {
	distro := gimlet.GetVars(r)["distro"]
	taskId := gimlet.GetVars(r)["task_id"]
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/task-queue/%s/%s", uis.Settings.Ui.UIv2Url, distro, taskId)
	}

	http.Redirect(w, r, newUILink, http.StatusTemporaryRedirect)
}

// legacyTaskHistoryPage redirects to the new UI waterfall page since there's no equivalent task history page
func (uis *UIServer) legacyTaskHistoryPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject(r.Context())

	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// There's no longer an equivalent Task History page on Spruce, so we have to link to a different page. Waterfall is used since
	// it is the most relevant.
	http.Redirect(w, r, fmt.Sprintf("%s/project/%s/waterfall", uis.Settings.Ui.UIv2Url, project.Identifier), http.StatusPermanentRedirect)
}

// legacyTaskPage redirects to the new UI task page
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

// legacyProjectsPage redirects to the new UI project settings page
func (uis *UIServer) legacyProjectsPage(w http.ResponseWriter, r *http.Request) {
	projectId := gimlet.GetVars(r)["project_id"]
	newUIProjectsLink := fmt.Sprintf("%s/project/%s/settings", uis.Settings.Ui.UIv2Url, projectId)
	http.Redirect(w, r, newUIProjectsLink, http.StatusPermanentRedirect)
}

