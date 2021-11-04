package service

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func (uis *UIServer) versionPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil || projCtx.Version == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if r.FormValue("redirect_spruce_users") == "true" {
		if u := gimlet.GetUser(r.Context()); u != nil {
			usr, ok := u.(*user.DBUser)
			if ok && usr.Settings.UseSpruceOptions.SpruceV1 {
				http.Redirect(w, r, fmt.Sprintf("%s/version/%s", uis.Settings.Ui.UIv2Url, projCtx.Version.Id), http.StatusTemporaryRedirect)
				return
			}
		}
	}

	// Set the config to blank to avoid writing it to the UI unnecessarily.
	projCtx.Version.Config = ""

	versionAsUI := uiVersion{
		Version:   *projCtx.Version,
		RepoOwner: projCtx.ProjectRef.Owner,
		Repo:      projCtx.ProjectRef.Repo,
	}

	if projCtx.Version.TriggerID != "" {
		var projectID, revision string
		if projCtx.Version.TriggerType == model.ProjectTriggerLevelTask {
			var upstreamTask *task.Task
			upstreamTask, err = task.FindOneId(projCtx.Version.TriggerID)
			if err != nil {
				http.Error(w, "error finding upstream task", http.StatusInternalServerError)
				return
			}
			if upstreamTask == nil {
				http.Error(w, "upstream task not found", http.StatusNotFound)
				return
			}
			revision = upstreamTask.Revision
			projectID = upstreamTask.Project
		} else if projCtx.Version.TriggerType == model.ProjectTriggerLevelBuild {
			var upstreamBuild *build.Build
			upstreamBuild, err = build.FindOneId(projCtx.Version.TriggerID)
			if err != nil {
				http.Error(w, "error finding upstream build", http.StatusInternalServerError)
				return
			}
			if upstreamBuild == nil {
				http.Error(w, "upstream build not found", http.StatusNotFound)
				return
			}
			revision = upstreamBuild.Revision
			projectID = upstreamBuild.Project
		}
		var project *model.ProjectRef
		project, err = model.FindBranchProjectRef(projectID)
		if err != nil {
			http.Error(w, "error finding upstream project", http.StatusInternalServerError)
			return
		}
		if project == nil {
			http.Error(w, "upstream project not found", http.StatusNotFound)
			return
		}
		versionAsUI.UpstreamData = &uiUpstreamData{
			Owner:       project.Owner,
			Repo:        project.Repo,
			Revision:    revision,
			ProjectName: project.DisplayName,
			TriggerID:   projCtx.Version.TriggerID,
			TriggerType: projCtx.Version.TriggerType,
		}
	}

	dbBuilds, err := build.Find(build.ByIds(projCtx.Version.BuildIds))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	currentUser := gimlet.GetUser(ctx)
	if projCtx.Patch != nil {
		versionAsUI.PatchInfo = &uiPatch{Patch: *projCtx.Patch}
		// diff builds for each build in the version
		var baseBuilds []build.Build
		baseBuilds, err = build.Find(build.ByRevisionWithSystemVersionRequester(projCtx.Version.Revision))
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
			var diff model.BuildStatusDiff
			diff, err = model.StatusDiffBuilds(
				baseBuildsByVariant[dbBuilds[i].BuildVariant],
				&dbBuilds[i],
			)
			if err != nil {
				http.Error(w,
					fmt.Sprintf("error calculating status diff for patch: %s", err),
					http.StatusInternalServerError)
				return
			}
			if diff.Name != "" {
				// append the tasks instead of the build for better usability
				diffs = append(diffs, diff.Tasks...)
			}
		}
		var baseVersion *model.Version
		baseVersion, err = model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(projCtx.Version.Identifier, projCtx.Version.Revision))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if baseVersion == nil {
			grip.Warningln("Could not find version for base commit of patch build: ", projCtx.Version.Id)
		}
		baseId := ""
		if baseVersion != nil {
			baseId = baseVersion.Id
		}
		versionAsUI.PatchInfo.BaseVersionId = baseId
		versionAsUI.PatchInfo.StatusDiffs = diffs
	}

	dbTasks, err := task.FindAll(task.ByVersion(projCtx.Version.Id).WithFields(task.StatusFields...))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	taskMap := task.TaskSliceToMap(dbTasks)
	failedTaskIds := []string{}
	uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
	for _, build := range dbBuilds {
		buildAsUI := uiBuild{Build: build}
		uiTasks := make([]uiTask, 0, len(build.Tasks))
		for _, taskCache := range build.Tasks {
			t, ok := taskMap[taskCache.Id]
			if !ok {
				grip.Error(message.Fields{
					"task_id": taskCache.Id,
					"version": projCtx.Version.Id,
					"request": gimlet.GetRequestID(ctx),
					"message": "build references task that does not exist",
				})
				continue
			}

			uiT := uiTask{
				Task: task.Task{
					Id:          t.Id,
					Activated:   t.Activated,
					StartTime:   t.StartTime,
					TimeTaken:   t.TimeTaken,
					Status:      t.Status,
					Details:     t.Details,
					DisplayName: t.DisplayName,
				}}

			if t.Status == evergreen.TaskStarted {
				uiT.ExpectedDuration = t.ExpectedDuration
			}
			uiTasks = append(uiTasks, uiT)
			buildAsUI.TaskStatusCount.IncrementStatus(t.Status, t.Details)
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
	err = addFailedTests(failedTaskIds, uiBuilds, taskMap)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	versionAsUI.Builds = uiBuilds

	versionAsUI.TimeTaken, versionAsUI.Makespan, err = projCtx.Version.GetTimeSpent()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	canEdit := (currentUser != nil) && (projCtx.Version.Requester != evergreen.MergeTestRequester)
	pluginContext := projCtx.ToPluginContext(uis.Settings, currentUser)
	pluginContent := getPluginDataAndHTML(uis, plugin.VersionPage, pluginContext)
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 && projCtx.Version.Requester == evergreen.PatchVersionRequester {
		newUILink = fmt.Sprintf("%s/version/%s", uis.Settings.Ui.UIv2Url, projCtx.Version.Id)
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Version       *uiVersion
		PluginContent pluginData
		CanEdit       bool
		JiraHost      string
		NewUILink     string
		ViewData
	}{
		NewUILink:     newUILink,
		Version:       &versionAsUI,
		PluginContent: pluginContent,
		CanEdit:       canEdit,
		JiraHost:      uis.Settings.Jira.Host,
		ViewData:      uis.GetCommonViewData(w, r, false, true)}, "base", "version.html", "base_angular.html", "menu.html")
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

	modifications := graphql.VersionModifications{}
	if err = utility.ReadJSON(util.NewRequestReader(r), &modifications); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpStatus, err := graphql.ModifyVersion(*projCtx.Version, *user, projCtx.ProjectRef, modifications)
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	// After the version has been modified, re-load it from DB and send back the up-to-date view
	// to the client.
	projCtx.Version, err = model.VersionFindOne(model.VersionById(projCtx.Version.Id))
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
	dbTasks, err := task.FindAll(task.ByVersion(projCtx.Version.Id).WithFields(task.StatusFields...))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	taskMap := task.TaskSliceToMap(dbTasks)

	uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
	for _, build := range dbBuilds {
		buildAsUI := uiBuild{Build: build}
		uiTasks := make([]uiTask, 0, len(build.Tasks))
		for _, taskCache := range build.Tasks {
			t, ok := taskMap[taskCache.Id]
			if !ok {
				grip.Error(message.Fields{
					"task_id": taskCache.Id,
					"version": projCtx.Version.Id,
					"request": gimlet.GetRequestID(r.Context()),
					"message": "build references task that does not exist",
				})
				continue
			}
			uiTasks = append(uiTasks,
				uiTask{
					Task: task.Task{Id: t.Id, Activated: t.Activated,
						StartTime: t.StartTime, TimeTaken: t.TimeTaken, Status: t.Status,
						Details: t.Details, DisplayName: t.DisplayName},
				})
			if t.Activated {
				versionAsUI.ActiveTasks++
			}
		}
		buildAsUI.Tasks = uiTasks
		uiBuilds = append(uiBuilds, buildAsUI)
	}
	versionAsUI.Builds = uiBuilds
	gimlet.WriteJSON(w, versionAsUI)
}

// addFailedTests fetches the tasks that failed from the database and attaches
// the associated failed tests to the uiBuilds.
func addFailedTests(failedTaskIds []string, uiBuilds []uiBuild, taskMap map[string]task.Task) error {
	if len(failedTaskIds) == 0 {
		return nil
	}

	failedTestsByTaskId := map[string][]string{}
	for _, tID := range failedTaskIds {
		failedTests := []string{}

		t, ok := taskMap[tID]
		if !ok {
			continue
		}
		for _, r := range t.LocalTestResults {
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
	data, err := model.VersionGetHistory(projCtx.Version.Id, 5)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	user := gimlet.GetUser(ctx)
	versions := make([]*uiVersion, 0, len(data))

	for _, version := range data {
		// Check whether the project associated with the particular version
		// is accessible to this user. If not, we exclude it from the version
		// history. This is done to hide the existence of the private project.
		if projCtx.ProjectRef.IsPrivate() && user == nil {
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
		dbTasks, err := task.FindAll(task.ByVersion(projCtx.Version.Id).WithFields(task.StatusFields...))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		taskMap := task.TaskSliceToMap(dbTasks)

		uiBuilds := make([]uiBuild, 0, len(projCtx.Version.BuildIds))
		for _, b := range dbBuilds {
			buildAsUI := uiBuild{Build: b}
			uiTasks := make([]uiTask, 0, len(b.Tasks))
			for _, taskCache := range b.Tasks {
				t, ok := taskMap[taskCache.Id]
				if !ok {
					grip.Error(message.Fields{
						"task_id": taskCache.Id,
						"version": projCtx.Version.Id,
						"request": gimlet.GetRequestID(r.Context()),
						"message": "build references task that does not exist",
					})
					continue
				}
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
	gimlet.WriteJSON(w, versions)
}

//versionFind redirects to the correct version page based on the gitHash and versionId given.
//It finds the version associated with the versionId and gitHash and redirects to /version/{version_id}.
func (uis *UIServer) versionFind(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	project := vars["project_id"]
	revision := vars["revision"]
	if len(revision) < 5 {
		http.Error(w, "revision not long enough: must be at least 5 characters", http.StatusBadRequest)
		return
	}
	id, err := model.GetIdForProject(project)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	foundVersions, err := model.VersionFind(model.VersionByProjectIdAndRevisionPrefix(id, revision).Limit(2))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if len(foundVersions) == 0 {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, fmt.Sprintf("Version Not Found: %v - %v", project, revision))
		return
	}
	if len(foundVersions) > 1 {
		gimlet.WriteJSONError(w, fmt.Sprintf("Multiple versions found: %v - %v", project, revision))
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/version/%v", foundVersions[0].Id), http.StatusFound)
}
