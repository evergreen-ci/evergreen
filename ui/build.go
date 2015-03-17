package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/model/version"
	"10gen.com/mci/plugin"
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

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
		RepoOwner:   projCtx.Project.Owner,
		Repo:        projCtx.Project.Repo,
		Version:     *projCtx.Version,
	}

	// add in the tasks
	tasks, err := model.FindTasksForBuild(projCtx.Build)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	uiTasks := make([]uiTask, 0, len(tasks))

	for _, task := range tasks {
		taskAsUI := uiTask{Task: task}
		uiTasks = append(uiTasks, taskAsUI)
	}

	buildAsUI.Tasks = sortUiTasks(uiTasks)

	if projCtx.Build.Requester == mci.PatchVersionRequester {
		//get patch associated with this build/version, if it exists
		patch, err := model.FindPatchByVersion(projCtx.Version.Id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if patch == nil {
			http.Error(w, fmt.Sprintf("No patch for version %v", projCtx.Version.Id), http.StatusNotFound)
			return
		}

		buildOnBaseCommit, err := projCtx.Build.FindBuildOnBaseCommit()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if buildOnBaseCommit == nil {
			mci.Logger.Logf(slogger.WARN,
				"Could not find build for base commit of patch build: %v", projCtx.Build.Id)
		}
		diffs := model.StatusDiffBuilds(buildOnBaseCommit, projCtx.Build)

		baseId := ""
		if buildOnBaseCommit != nil {
			baseId = buildOnBaseCommit.Id
		}

		buildAsUI.PatchInfo = &uiPatch{Patch: *patch, BaseBuildId: baseId, StatusDiffs: diffs.Tasks}
	}

	// set data for plugin data function injection
	pluginContext := projCtx.ToPluginContext(uis.MCISettings, GetUser(r))
	pluginContent := getPluginDataAndHTML(uis, plugin.BuildPage, pluginContext)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData   projectContext
		User          *model.DBUser
		Flashes       []interface{}
		Build         *uiBuild
		PluginContent pluginData
	}{projCtx, GetUser(r), flashes, buildAsUI, pluginContent}, "base", "build.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyBuild(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

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
		Action   string `json:"action"`
		Active   bool   `json:"active"`
		Abort    bool   `json:"abort"`
		Priority string `json:"priority"`
	}{}
	err = json.Unmarshal(reqBody, &putParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// determine what action needs to be taken
	switch putParams.Action {
	case "abort":
		if err := model.AbortBuild(projCtx.Build.Id); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
		model.RefreshTasksCache(projCtx.Build.Id)
		uis.WriteJSON(w, http.StatusOK, "Successfully marked tasks as aborted")
		return
	case "set_priority":
		priority, err := strconv.Atoi(putParams.Priority)
		if err != nil {
			http.Error(w, "Bad priority value; must be int", http.StatusBadRequest)
			return
		}
		err = model.SetBuildPriority(projCtx.Build.Id, priority)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error setting priority on build %v", projCtx.Build.Id),
				http.StatusInternalServerError)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Priority for build set to %v.", priority)))
		uis.WriteJSON(w, http.StatusOK, "Successfully set priority")
		return
	case "set_active":
		err := model.SetBuildActivation(projCtx.Build.Id, putParams.Active)
		//TODO update version builds cache?
		if err != nil {
			http.Error(w, fmt.Sprintf("Error marking build %v as activated=%v", projCtx.Build.Id, putParams.Active),
				http.StatusInternalServerError)
			return
		}
		if putParams.Active {
			PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Build scheduled."))
		} else {
			PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Build unscheduled."))
		}
		uis.WriteJSON(w, http.StatusOK, fmt.Sprintf("Successfully marked build as active=%v", putParams.Active))
		return
	case "restart":
		if err := model.RestartBuild(projCtx.Build.Id, putParams.Abort); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting build %v", projCtx.Build.Id), http.StatusInternalServerError)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Build %v restarted.", projCtx.Build.Id)))
		uis.WriteJSON(w, http.StatusOK, fmt.Sprintf("Successfully restarted build", putParams.Active))
		return
	default:
		uis.WriteJSON(w, http.StatusBadRequest, "Unrecognized action")
		return
	}
}

func (uis *UIServer) lastGreenHandler(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	// queryParams should list build variants, example:
	// GET /ui/json/last_green/mongodb-mongo-master?linux-64=1&windows-64=1
	queryParams := r.URL.Query()

	if projCtx.Project == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	// Make sure all query params are valid build variants and put them in an array
	var buildVariantNames []string
	for key, _ := range queryParams {
		if projCtx.Project.FindBuildVariant(key) != nil {
			buildVariantNames = append(buildVariantNames, key)
		} else {
			http.Error(w, "build variant not found", http.StatusNotFound)
			return
		}
	}

	// Get latest version for which all the given build variants passed.
	version, err := model.FindLastPassingVersionForBuildVariants(*projCtx.Project,
		buildVariantNames)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if version == nil {
		http.Error(w, "Couldn't find latest green version", http.StatusNotFound)
		return
	}

	uis.WriteJSON(w, http.StatusOK, version)
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
			http.Error(w, fmt.Sprintf("error getting last successful build version: %v", err), http.StatusInternalServerError)
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
