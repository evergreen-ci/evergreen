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
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"github.com/tychoish/grip"
)

// getUiTaskCache takes a build object and returns a slice of
// uiTask objects suitable for front-end
func getUiTaskCache(build *build.Build) ([]uiTask, error) {
	tasks, err := task.Find(task.ByBuildId(build.Id))
	if err != nil {
		return nil, err
	}

	idToTask := make(map[string]task.Task)
	for _, task := range tasks {
		idToTask[task.Id] = task
	}

	// Insert the tasks in the same order as the task cache
	uiTasks := make([]uiTask, 0, len(build.Tasks))
	for _, taskCache := range build.Tasks {
		taskAsUI := uiTask{Task: idToTask[taskCache.Id]}
		uiTasks = append(uiTasks, taskAsUI)
	}
	return uiTasks, nil
}

func (uis *UIServer) buildPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Build == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}

	flashes := PopFlashes(uis.CookieStore, r, w)

	buildAsUI := &uiBuild{
		Build:       *projCtx.Build,
		CurrentTime: time.Now().UnixNano(),
		Elapsed:     time.Now().Sub(projCtx.Build.StartTime),
		RepoOwner:   projCtx.ProjectRef.Owner,
		Repo:        projCtx.ProjectRef.Repo,
		Version:     *projCtx.Version,
	}

	uiTasks, err := getUiTaskCache(projCtx.Build)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	buildAsUI.Tasks = uiTasks

	if projCtx.Build.Requester == evergreen.PatchVersionRequester {
		buildOnBaseCommit, err := projCtx.Build.FindBuildOnBaseCommit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if buildOnBaseCommit == nil {
			grip.Warningln("Could not find build for base commit of patch build:",
				projCtx.Build.Id)
		}
		diffs := model.StatusDiffBuilds(buildOnBaseCommit, projCtx.Build)

		baseId := ""
		if buildOnBaseCommit != nil {
			baseId = buildOnBaseCommit.Id
		}
		buildAsUI.PatchInfo = &uiPatch{Patch: *projCtx.Patch, BaseBuildId: baseId, StatusDiffs: diffs.Tasks}
	}

	// set data for plugin data function injection
	pluginContext := projCtx.ToPluginContext(uis.Settings, GetUser(r))
	pluginContent := getPluginDataAndHTML(uis, plugin.BuildPage, pluginContext)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData   projectContext
		User          *user.DBUser
		Flashes       []interface{}
		Build         *uiBuild
		PluginContent pluginData
	}{projCtx, GetUser(r), flashes, buildAsUI, pluginContent}, "base", "build.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyBuild(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	user := MustHaveUser(r)

	if projCtx.Build == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

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
		if err := model.AbortBuild(projCtx.Build.Id, user.Id); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
		model.RefreshTasksCache(projCtx.Build.Id)
	case "set_priority":
		priority, err := strconv.ParseInt(putParams.Priority, 10, 64)
		if err != nil {
			http.Error(w, "Bad priority value; must be int", http.StatusBadRequest)
			return
		}
		if priority > evergreen.MaxTaskPriority {
			if !uis.isSuperUser(user) {
				http.Error(w, fmt.Sprintf("Insufficient access to set priority %v, can only set prior less than or equal to %v", priority, evergreen.MaxTaskPriority),
					http.StatusBadRequest)
				return
			}
		}
		err = model.SetBuildPriority(projCtx.Build.Id, priority)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error setting priority on build %v", projCtx.Build.Id),
				http.StatusInternalServerError)
			return
		}
	case "set_active":
		err := model.SetBuildActivation(projCtx.Build.Id, putParams.Active, user.Id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error marking build %v as activated=%v", projCtx.Build.Id, putParams.Active),
				http.StatusInternalServerError)
			return
		}
	case "restart":
		if err := model.RestartBuild(projCtx.Build.Id, putParams.TaskIds, putParams.Abort, user.Id); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
	default:
		uis.WriteJSON(w, http.StatusBadRequest, "Unrecognized action")
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
		Elapsed:     time.Now().Sub(projCtx.Build.StartTime),
		RepoOwner:   projCtx.ProjectRef.Owner,
		Repo:        projCtx.ProjectRef.Repo,
		Version:     *projCtx.Version,
	}

	uiTasks, err := getUiTaskCache(projCtx.Build)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	updatedBuild.Tasks = uiTasks
	uis.WriteJSON(w, http.StatusOK, updatedBuild)
}

func (uis *UIServer) buildHistory(w http.ResponseWriter, r *http.Request) {
	buildId := mux.Vars(r)["build_id"]

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
		v, err := version.FindOne(version.ById(builds[i].Version))
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
			Elapsed:     time.Now().Sub(builds[i].StartTime),
			RepoOwner:   v.Owner,
			Repo:        v.Repo,
			Version:     *v,
		}
	}

	lastSuccess, err := getBuildVariantHistoryLastSuccess(buildId)
	if err == nil && lastSuccess != nil {
		v, err := version.FindOne(version.ById(lastSuccess.Version))
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
			Elapsed:     time.Now().Sub(lastSuccess.StartTime),
			RepoOwner:   v.Owner,
			Repo:        v.Repo,
			Version:     *v,
		}
	}

	uis.WriteJSON(w, http.StatusOK, history)
}
