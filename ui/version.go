package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"net/http"
)

func (uis *UIServer) versionPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil || projCtx.Version == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Set the config to blank to avoid writing it to the UI unnecessarily.
	projCtx.Version.Config = ""

	versionAsUI := uiVersion{
		Version:   *projCtx.Version,
		RepoOwner: projCtx.ProjectRef.Owner,
		Repo:      projCtx.ProjectRef.Repo,
	}

	dbBuilds, err := build.Find(build.ByIds(projCtx.Version.BuildIds))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if projCtx.Patch != nil {
		versionAsUI.PatchInfo = &uiPatch{Patch: *projCtx.Patch}
		// diff builds for each build in the version
		baseBuilds, err := build.Find(build.ByRevision(projCtx.Version.Revision))
		if err != nil {
			http.Error(w,
				fmt.Sprintf("error loading base builds for patch: %v", err),
				http.StatusInternalServerError)
			return
		}
		baseBuildsByVariant := map[string]*build.Build{}
		for i := range baseBuilds {
			baseBuildsByVariant[baseBuilds[i].BuildVariant] = &baseBuilds[i]
		}
		// diff all patch builds with their original build
		diffs := []model.TaskStatusDiff{}
		for i := range dbBuilds {
			diff := model.StatusDiffBuilds(
				baseBuildsByVariant[dbBuilds[i].BuildVariant],
				&dbBuilds[i],
			)
			if diff.Name != "" {
				// append the tasks instead of the build for better usability
				diffs = append(diffs, diff.Tasks...)
			}
		}
		versionAsUI.PatchInfo.StatusDiffs = diffs
	}

	uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
	for _, build := range dbBuilds {
		buildAsUI := uiBuild{Build: build}

		uiTasks := make([]uiTask, 0, len(build.Tasks))
		for _, task := range build.Tasks {
			uiTasks = append(uiTasks,
				uiTask{Task: model.Task{Id: task.Id, Activated: task.Activated,
					Status: task.Status, Details: task.StatusDetails, DisplayName: task.DisplayName}})
			if task.Activated {
				versionAsUI.ActiveTasks++
			}
		}
		uiTasks = sortUiTasks(uiTasks)
		buildAsUI.Tasks = uiTasks
		uiBuilds = append(uiBuilds, buildAsUI)
	}
	versionAsUI.Builds = uiBuilds

	pluginContext := projCtx.ToPluginContext(uis.Settings, GetUser(r))
	pluginContent := getPluginDataAndHTML(uis, plugin.VersionPage, pluginContext)

	flashes := PopFlashes(uis.CookieStore, r, w)
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData   projectContext
		User          *user.DBUser
		Flashes       []interface{}
		Version       *uiVersion
		PluginContent pluginData
	}{projCtx, GetUser(r), flashes, &versionAsUI, pluginContent}, "base",
		"version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyVersion(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil || projCtx.Version == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	jsonMap := struct {
		Action   string `json:"action"`
		Active   bool   `json:"active"`
		Abort    bool   `json:"abort"`
		Priority int    `json:"priority"`
	}{}
	err := util.ReadJSONInto(r.Body, &jsonMap)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// determine what action needs to be taken
	switch jsonMap.Action {
	case "set_active":
		if jsonMap.Abort {
			if err = model.AbortVersion(projCtx.Version.Id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else {
				msg := NewSuccessFlash("All tasks in this version are now aborting.")
				PushFlash(uis.CookieStore, r, w, msg)
			}
		}
		if err = model.SetVersionActivation(projCtx.Version.Id, jsonMap.Active); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			if !jsonMap.Abort { // don't add a msg if we already added one for abort
				var msg flashMessage
				if jsonMap.Active {
					msg = NewSuccessFlash("All tasks in this version are now scheduled.")
				} else {
					msg = NewSuccessFlash("All tasks in this version are now unscheduled.")
				}
				PushFlash(uis.CookieStore, r, w, msg)
			}
		}
		uis.WriteJSON(w, http.StatusOK, projCtx.Version)
		return
	case "set_priority":
		if err = model.SetVersionPriority(projCtx.Version.Id, jsonMap.Priority); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		msg := NewSuccessFlash(fmt.Sprintf("Priority for all tasks in this version set to %v.", jsonMap.Priority))
		PushFlash(uis.CookieStore, r, w, msg)
		uis.WriteJSON(w, http.StatusOK, projCtx.Version)
		return
	default:
		uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Unrecognized action: %v", jsonMap.Action))
		return
	}
	return
}

func (uis *UIServer) versionHistory(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	data, err := getVersionHistory(projCtx.Version.Id, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user := GetUser(r)
	versions := make([]*uiVersion, 0, len(data))

	for _, version := range data {
		// Check whether the project associated with the particular version
		// is accessible to this user. If not, we exclude it from the version
		// history. This is done to hide the existence of the private project.
		if projCtx.ProjectRef.Private && user == nil {
			continue
		}

		versionAsUI := uiVersion{
			Version:   version,
			RepoOwner: projCtx.ProjectRef.Owner,
			Repo:      projCtx.ProjectRef.Repo,
		}
		versions = append(versions, &versionAsUI)

		dbBuilds, err := build.Find(build.ByIds(version.BuildIds))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
		for _, b := range dbBuilds {
			buildAsUI := uiBuild{Build: b}
			uiTasks := make([]uiTask, 0, len(b.Tasks))
			for _, task := range b.Tasks {
				uiTasks = append(uiTasks,
					uiTask{
						Task: model.Task{
							Id:          task.Id,
							Status:      task.Status,
							Activated:   task.Activated,
							DisplayName: task.DisplayName,
						},
					})
				if task.Activated {
					versionAsUI.ActiveTasks++
				}
			}
			uiTasks = sortUiTasks(uiTasks)
			buildAsUI.Tasks = uiTasks
			uiBuilds = append(uiBuilds, buildAsUI)
		}
		versionAsUI.Builds = uiBuilds
	}
	uis.WriteJSON(w, http.StatusOK, versions)
}
