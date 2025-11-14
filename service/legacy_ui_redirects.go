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
	project, err := projCtx.GetProject(r.Context())

	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// There's no longer an equivalent Task History page on Spruce, so we have to link to a different page. Waterfall is used since
	// it is the most relevant.
	http.Redirect(w, r, fmt.Sprintf("%s/project/%s/waterfall", uis.Settings.Ui.UIv2Url, project.Identifier), http.StatusPermanentRedirect)
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

func (uis *UIServer) legacyUserSettingsPage(w http.ResponseWriter, r *http.Request) {
	newUILink := fmt.Sprintf("%s/preferences/profile", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, newUILink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyNotificationsPage(w http.ResponseWriter, r *http.Request) {
	newUILink := fmt.Sprintf("%s/preferences/notifications", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, newUILink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyHostsPage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/hosts", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyHostPage(w http.ResponseWriter, r *http.Request) {
	hostId := gimlet.GetVars(r)["host_id"]
	spruceLink := fmt.Sprintf("%s/host/%s", uis.Settings.Ui.UIv2Url, hostId)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyUserPatchesPage(w http.ResponseWriter, r *http.Request) {
	userId := gimlet.GetVars(r)["user_id"]
	spruceLink := fmt.Sprintf("%s/user/%s/patches", uis.Settings.Ui.UIv2Url, userId)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyMyPatchesPage(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	spruceLink := fmt.Sprintf("%s/user/%s/patches", uis.Settings.Ui.UIv2Url, user.Username())
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyProjectPatchesPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject(r.Context())
	if err != nil || project == nil {
		http.Redirect(w, r, uis.Settings.Ui.UIv2Url, http.StatusPermanentRedirect)
		return
	}
	spruceLink := fmt.Sprintf("%s/project/%s/patches", uis.Settings.Ui.UIv2Url, project.Identifier)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyPatchesPage(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	// There's no equivalent /patches page on Spruce, so just redirect to user patches.
	spruceLink := fmt.Sprintf("%s/user/%s/patches", uis.Settings.Ui.UIv2Url, user.Username())
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacySpawnHostPage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/spawn/host", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacySpawnVolumePage(w http.ResponseWriter, r *http.Request) {
	spruceLink := fmt.Sprintf("%s/spawn/volume", uis.Settings.Ui.UIv2Url)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyVersionPage(w http.ResponseWriter, r *http.Request) {
	versionId := gimlet.GetVars(r)["version_id"]
	spruceLink := fmt.Sprintf("%s/version/%s", uis.Settings.Ui.UIv2Url, versionId)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyPatchPage(w http.ResponseWriter, r *http.Request) {
	patchId := gimlet.GetVars(r)["patch_id"]
	// Spruce is able to determine if a /patch link should redirect to the configure or version page
	spruceLink := fmt.Sprintf("%s/patch/%s", uis.Settings.Ui.UIv2Url, patchId)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}

func (uis *UIServer) legacyBuildBaronPage(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	executionStr := vars["execution"]

	// Default to execution 0 if not specified
	execution := 0
	if executionStr != "" {
		var err error
		execution, err = strconv.Atoi(executionStr)
		if err != nil {
			execution = 0
		}
	}

	spruceLink := fmt.Sprintf("%s/task/%s?execution=%d", uis.Settings.Ui.UIv2Url, taskId, execution)
	http.Redirect(w, r, spruceLink, http.StatusPermanentRedirect)
}
