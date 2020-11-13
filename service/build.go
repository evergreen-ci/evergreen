package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// getUiTaskCache takes a build object and returns a slice of
// uiTask objects suitable for front-end
func getUiTaskCache(build *build.Build, tasks []task.Task) ([]uiTask, error) {
	idToTask := task.TaskSliceToMap(tasks)

	// Insert the tasks in the same order as the task cache
	uiTasks := make([]uiTask, 0, len(build.Tasks))
	for _, taskCache := range build.Tasks {
		t, ok := idToTask[taskCache.Id]
		if !ok {
			continue
		}

		taskAsUI := uiTask{Task: t}
		uiTasks = append(uiTasks, taskAsUI)
	}

	return uiTasks, nil
}

func (uis *UIServer) buildPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if r.FormValue("redirect_spruce_users") == "true" {
		user := MustHaveUser(r)
		if user.Settings.UseSpruceOptions.SpruceV1 {
			http.Redirect(w, r, fmt.Sprintf("%s/version/%s?variant=%s", uis.Settings.Ui.UIv2Url, projCtx.Version.Id, projCtx.Build.DisplayName), http.StatusTemporaryRedirect)
			return
		}
	}

	if projCtx.Build == nil || projCtx.Version == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New("not found"))
		return
	}
	buildAsUI := &uiBuild{
		Build:       *projCtx.Build,
		CurrentTime: time.Now().UnixNano(),
		Elapsed:     time.Since(projCtx.Build.StartTime),
		Version:     *projCtx.Version,
	}

	if projCtx.ProjectRef != nil {
		buildAsUI.RepoOwner = projCtx.ProjectRef.Owner
		buildAsUI.Repo = projCtx.ProjectRef.Repo
	}

	tasks, err := task.FindAllFirstExecution(task.ByBuildId(projCtx.Build.Id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "can't get tasks for build"))
		return
	}
	uiTasks, err := getUiTaskCache(projCtx.Build, tasks)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	buildAsUI.Tasks = uiTasks
	buildAsUI.TimeTaken, buildAsUI.Makespan = task.GetTimeSpent(tasks)

	if projCtx.Build.TriggerID != "" {
		var projectName string
		projectName, err = model.GetUpstreamProjectName(projCtx.Build.TriggerID, projCtx.Build.TriggerType)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		buildAsUI.UpstreamData = &uiUpstreamData{
			ProjectName: projectName,
			TriggerID:   projCtx.Build.TriggerID,
			TriggerType: projCtx.Build.TriggerType,
		}
	}

	if evergreen.IsPatchRequester(projCtx.Build.Requester) {
		buildOnBaseCommit, err := projCtx.Build.FindBuildOnBaseCommit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if buildOnBaseCommit == nil {
			grip.Warningln("Could not find build for base commit of patch build:",
				projCtx.Build.Id)
		}
		diffs, err := model.StatusDiffBuilds(buildOnBaseCommit, projCtx.Build)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		baseId := ""
		if buildOnBaseCommit != nil {
			baseId = buildOnBaseCommit.Id
		}
		buildAsUI.PatchInfo = &uiPatch{Patch: *projCtx.Patch, BaseBuildId: baseId, StatusDiffs: diffs.Tasks}
	}

	ctx := r.Context()
	user := gimlet.GetUser(ctx)

	// set data for plugin data function injection
	pluginContext := projCtx.ToPluginContext(uis.Settings, user)
	pluginContent := getPluginDataAndHTML(uis, plugin.BuildPage, pluginContext)

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Build         *uiBuild
		PluginContent pluginData
		JiraHost      string
		ViewData
	}{buildAsUI, pluginContent, uis.Settings.Jira.Host, uis.GetCommonViewData(w, r, false, true)}, "base", "build.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyBuild(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	user := MustHaveUser(r)

	if projCtx.Build == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	body := util.NewRequestReader(r)
	defer body.Close()
	reqBody, err := ioutil.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	putParams := struct {
		Action   string   `json:"action"`
		Active   bool     `json:"active"`
		Abort    bool     `json:"abort"`
		Priority string   `json:"priority"`
		TaskIds  []string `json:"taskIds"`
	}{}
	err = json.Unmarshal(reqBody, &putParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// determine what action needs to be taken
	switch putParams.Action {
	case "abort":
		if err = model.AbortBuild(projCtx.Build.Id, user.Id); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
		if err = model.RefreshTasksCache(projCtx.Build.Id); err != nil {
			http.Error(w, fmt.Sprintf("problem refreshing tasks cache %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
		if projCtx.Build.Requester == evergreen.MergeTestRequester {
			_, err = commitqueue.RemoveCommitQueueItemForVersion(projCtx.ProjectRef.Id,
				projCtx.ProjectRef.CommitQueue.PatchType, projCtx.Build.Version, user.Id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	case "set_priority":
		var priority int64
		priority, err = strconv.ParseInt(putParams.Priority, 10, 64)
		if err != nil {
			http.Error(w, "Bad priority value; must be int", http.StatusBadRequest)
			return
		}
		if priority > evergreen.MaxTaskPriority {
			requiredPermission := gimlet.PermissionOpts{
				Resource:      projCtx.ProjectRef.Id,
				ResourceType:  "project",
				Permission:    evergreen.PermissionTasks,
				RequiredLevel: evergreen.TasksAdmin.Value,
			}
			if !user.HasPermission(requiredPermission) {
				http.Error(w, fmt.Sprintf("Insufficient access to set priority %v, can only set prior less than or equal to %v", priority, evergreen.MaxTaskPriority),
					http.StatusUnauthorized)
				return
			}
		}
		err = model.SetBuildPriority(projCtx.Build.Id, priority, user.Id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error setting priority on build %v", projCtx.Build.Id),
				http.StatusInternalServerError)
			return
		}
	case "set_active":
		err = model.SetBuildActivation(projCtx.Build.Id, putParams.Active, user.Id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error marking build %v as activated=%v", projCtx.Build.Id, putParams.Active),
				http.StatusInternalServerError)
			return
		}
		if !putParams.Active && putParams.Abort {
			if err = task.AbortBuild(projCtx.Build.Id, task.AbortInfo{User: user.Id}); err != nil {
				http.Error(w, "Error unscheduling tasks", http.StatusInternalServerError)
				return
			}
		}
		if !putParams.Active && projCtx.Build.Requester == evergreen.MergeTestRequester {
			_, err = commitqueue.RemoveCommitQueueItemForVersion(projCtx.ProjectRef.Id,
				projCtx.ProjectRef.CommitQueue.PatchType, projCtx.Build.Version, user.Id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	case "restart":
		if err = model.RestartBuild(projCtx.Build.Id, putParams.TaskIds, putParams.Abort, user.Id); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
	default:
		gimlet.WriteJSONError(w, "Unrecognized action")
		return
	}

	// After updating the build, fetch updated version to serve back to client
	projCtx.Build, err = build.FindOne(build.ById(projCtx.Build.Id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	updatedBuild := uiBuild{
		Build:       *projCtx.Build,
		CurrentTime: time.Now().UnixNano(),
		Elapsed:     time.Since(projCtx.Build.StartTime),
		RepoOwner:   projCtx.ProjectRef.Owner,
		Repo:        projCtx.ProjectRef.Repo,
		Version:     *projCtx.Version,
	}

	tasks, err := task.FindAllFirstExecution(task.ByBuildId(projCtx.Build.Id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "can't get tasks for build"))
		return
	}
	uiTasks, err := getUiTaskCache(projCtx.Build, tasks)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	updatedBuild.Tasks = uiTasks
	updatedBuild.TimeTaken, updatedBuild.Makespan = task.GetTimeSpent(tasks)

	gimlet.WriteJSON(w, updatedBuild)
}

func (uis *UIServer) buildHistory(w http.ResponseWriter, r *http.Request) {
	buildId := gimlet.GetVars(r)["build_id"]

	before, err := getIntValue(r, "before", 3)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid param 'before': %v", r.FormValue("before")), http.StatusBadRequest)
		return
	}

	after, err := getIntValue(r, "after", 3)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid param 'after': %v", r.FormValue("after")), http.StatusBadRequest)
		return
	}

	builds, err := getBuildVariantHistory(buildId, before, after)
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting build history: %v", err), http.StatusInternalServerError)
		return
	}

	history := &struct {
		Builds      []*uiBuild `json:"builds"`
		LastSuccess *uiBuild   `json:"lastSuccess"`
	}{}

	history.Builds = make([]*uiBuild, len(builds))
	for i := 0; i < len(builds); i++ {
		var v *model.Version
		v, err = model.VersionFindOne(model.VersionById(builds[i].Version))
		if err != nil {
			http.Error(w, fmt.Sprintf("error getting version for build %v: %v", builds[i].Id, err), http.StatusInternalServerError)
			return
		}
		if v == nil {
			http.Error(w, fmt.Sprintf("no version found for build %v", builds[i].Id), http.StatusNotFound)
			return
		}
		history.Builds[i] = &uiBuild{
			Build:       builds[i],
			CurrentTime: time.Now().UnixNano(),
			Elapsed:     time.Since(builds[i].StartTime),
			RepoOwner:   v.Owner,
			Repo:        v.Repo,
			Version:     *v,
		}
	}

	lastSuccess, err := getBuildVariantHistoryLastSuccess(buildId)
	if err == nil && lastSuccess != nil {
		v, err := model.VersionFindOne(model.VersionById(lastSuccess.Version))
		if err != nil {
			http.Error(
				w, fmt.Sprintf("error getting last successful build version: %v", err),
				http.StatusInternalServerError)
			return
		}
		if v == nil {
			http.Error(w, fmt.Sprintf("no version '%v' found", lastSuccess.Version), http.StatusNotFound)
			return
		}
		history.LastSuccess = &uiBuild{
			Build:       *lastSuccess,
			CurrentTime: time.Now().UnixNano(),
			Elapsed:     time.Since(lastSuccess.StartTime),
			RepoOwner:   v.Owner,
			Repo:        v.Repo,
			Version:     *v,
		}
	}

	gimlet.WriteJSON(w, history)
}
