package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func (uis *UIServer) versionPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil || projCtx.Version == nil {
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
	var canEditPatch bool
	currentUser := GetUser(r)
	if projCtx.Patch != nil {
		canEditPatch = uis.canEditPatch(currentUser, projCtx.Patch)
		versionAsUI.PatchInfo = &uiPatch{Patch: *projCtx.Patch}
		// diff builds for each build in the version
		var baseBuilds []build.Build
		baseBuilds, err = build.Find(build.ByRevision(projCtx.Version.Revision))
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
		var baseVersion *version.Version
		baseVersion, err = version.FindOne(version.BaseVersionFromPatch(projCtx.Version.Identifier, projCtx.Version.Revision))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if baseVersion == nil {
			grip.Warningln("Could not find version for base commmit of patch build: ", projCtx.Version.Id)
		}
		baseId := ""
		if baseVersion != nil {
			baseId = baseVersion.Id
		}
		versionAsUI.PatchInfo.BaseVersionId = baseId
		versionAsUI.PatchInfo.StatusDiffs = diffs
	}

	failedTaskIds := []string{}
	uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
	for _, build := range dbBuilds {
		buildAsUI := uiBuild{Build: build}

		uiTasks := make([]uiTask, 0, len(build.Tasks))
		for _, t := range build.Tasks {
			uiT := uiTask{
				Task: task.Task{
					Id:          t.Id,
					Activated:   t.Activated,
					StartTime:   t.StartTime,
					TimeTaken:   t.TimeTaken,
					Status:      t.Status,
					Details:     t.StatusDetails,
					DisplayName: t.DisplayName,
				}}
			if t.Status == evergreen.TaskStarted {
				var taskFromDb *task.Task
				taskFromDb, err = task.FindOne(task.ById(t.Id))
				if err != nil {
					uis.LoggedError(w, r, http.StatusInternalServerError, err)
				}
				uiT.ExpectedDuration = taskFromDb.ExpectedDuration
			}
			uiTasks = append(uiTasks, uiT)
			buildAsUI.TaskStatusCount.incrementStatus(t.Status, t.StatusDetails)
			if t.Status == evergreen.TaskFailed {
				failedTaskIds = append(failedTaskIds, t.Id)
			}
			if t.Activated {
				versionAsUI.ActiveTasks++
			}
		}
		buildAsUI.Tasks = uiTasks
		uiBuilds = append(uiBuilds, buildAsUI)
	}
	err = addFailedTests(failedTaskIds, uiBuilds)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	versionAsUI.Builds = uiBuilds

	pluginContext := projCtx.ToPluginContext(uis.Settings, GetUser(r))
	pluginContent := getPluginDataAndHTML(uis, plugin.VersionPage, pluginContext)

	uis.WriteHTML(w, http.StatusOK, struct {
		Version       *uiVersion
		PluginContent pluginData
		CanEdit       bool
		JiraHost      string
		ViewData
	}{&versionAsUI, pluginContent, canEditPatch,
		uis.Settings.Jira.Host, uis.GetCommonViewData(w, r, false, true)}, "base", "version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyVersion(w http.ResponseWriter, r *http.Request) {
	var err error

	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil || projCtx.Version == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	user := MustHaveUser(r)

	jsonMap := struct {
		Action   string   `json:"action"`
		Active   bool     `json:"active"`
		Abort    bool     `json:"abort"`
		Priority int64    `json:"priority"`
		TaskIds  []string `json:"task_ids"`
	}{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), &jsonMap); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// determine what action needs to be taken
	switch jsonMap.Action {
	case "restart":
		if err = model.RestartVersion(projCtx.Version.Id, jsonMap.TaskIds, jsonMap.Abort, user.Id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "set_active":
		if jsonMap.Abort {
			if err = model.AbortVersion(projCtx.Version.Id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if err = model.SetVersionActivation(projCtx.Version.Id, jsonMap.Active, user.Id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "set_priority":
		if jsonMap.Priority > evergreen.MaxTaskPriority {
			if !uis.isSuperUser(user) {
				http.Error(w, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", jsonMap.Priority, evergreen.MaxTaskPriority),
					http.StatusBadRequest)
				return
			}
		}
		if err = model.SetVersionPriority(projCtx.Version.Id, jsonMap.Priority); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Unrecognized action: %v", jsonMap.Action))
		return
	}

	// After the version has been modified, re-load it from DB and send back the up-to-date view
	// to the client.
	projCtx.Version, err = version.FindOne(version.ById(projCtx.Version.Id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	versionAsUI := uiVersion{
		Version:   *projCtx.Version,
		RepoOwner: projCtx.ProjectRef.Owner,
		Repo:      projCtx.ProjectRef.Repo,
	}
	dbBuilds, err := build.Find(build.ByIds(projCtx.Version.BuildIds))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
	for _, build := range dbBuilds {
		buildAsUI := uiBuild{Build: build}
		uiTasks := make([]uiTask, 0, len(build.Tasks))
		for _, t := range build.Tasks {
			uiTasks = append(uiTasks,
				uiTask{
					Task: task.Task{Id: t.Id, Activated: t.Activated,
						StartTime: t.StartTime, TimeTaken: t.TimeTaken, Status: t.Status,
						Details: t.StatusDetails, DisplayName: t.DisplayName},
				})
			if t.Activated {
				versionAsUI.ActiveTasks++
			}
		}
		buildAsUI.Tasks = uiTasks
		uiBuilds = append(uiBuilds, buildAsUI)
	}
	versionAsUI.Builds = uiBuilds
	uis.WriteJSON(w, http.StatusOK, versionAsUI)
}

// addFailedTests fetches the tasks that failed from the database and attaches
// the associated failed tests to the uiBuilds.
func addFailedTests(failedTaskIds []string, uiBuilds []uiBuild) error {
	if len(failedTaskIds) == 0 {
		return nil
	}
	failedTasks, err := task.Find(task.ByIds(failedTaskIds))
	if err != nil {
		return errors.Wrap(err, "error fetching failed tasks")
	}

	failedTestsByTaskId := map[string][]string{}
	for _, t := range failedTasks {
		failedTests := []string{}
		for _, r := range t.TestResults {
			if r.Status == evergreen.TestFailedStatus {
				failedTests = append(failedTests, r.TestFile)
			}
		}
		failedTestsByTaskId[t.Id] = failedTests
	}
	for i, build := range uiBuilds {
		for j, t := range build.Tasks {
			if len(failedTestsByTaskId[t.Task.Id]) != 0 {
				uiBuilds[i].Tasks[j].FailedTestNames = append(uiBuilds[i].Tasks[j].FailedTestNames, failedTestsByTaskId[t.Task.Id]...)
				sort.Strings(uiBuilds[i].Tasks[j].FailedTestNames)
			}
		}
	}
	return nil
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
			for _, t := range b.Tasks {
				uiTasks = append(uiTasks,
					uiTask{
						Task: task.Task{
							Id:          t.Id,
							Status:      t.Status,
							Activated:   t.Activated,
							DisplayName: t.DisplayName,
						},
					})
				if t.Activated {
					versionAsUI.ActiveTasks++
				}
			}
			buildAsUI.Tasks = uiTasks
			uiBuilds = append(uiBuilds, buildAsUI)
		}
		versionAsUI.Builds = uiBuilds
	}
	uis.WriteJSON(w, http.StatusOK, versions)
}

//versionFind redirects to the correct version page based on the gitHash and versionId given.
//It finds the version associated with the versionId and gitHash and redirects to /version/{version_id}.
func (uis *UIServer) versionFind(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["project_id"]
	revision := mux.Vars(r)["revision"]
	if len(revision) < 5 {
		http.Error(w, "revision not long enough: must be at least 5 characters", http.StatusBadRequest)
		return
	}
	foundVersions, err := version.Find(version.ByProjectIdAndRevisionPrefix(id, revision).Limit(2))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if len(foundVersions) == 0 {
		uis.WriteJSON(w, http.StatusNotFound, fmt.Sprintf("Version Not Found: %v - %v", id, revision))
		return
	}
	if len(foundVersions) > 1 {
		uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Multiple versions found: %v - %v", id, revision))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/version/%v", foundVersions[0].Id), http.StatusFound)
}
