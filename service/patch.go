package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type patchVariantsTasksRequest struct {
	VariantsTasks []patch.VariantTasks `json:"variants_tasks,omitempty"` // new format
	Variants      []string             `json:"variants"`                 // old format
	Tasks         []string             `json:"tasks"`                    // old format
	Description   string               `json:"description"`
}

func (uis *UIServer) patchPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	currentUser := MustHaveUser(r)

	var versionAsUI *uiVersion
	if projCtx.Version != nil { // Patch is already finalized
		versionAsUI = &uiVersion{
			Version:   *projCtx.Version,
			RepoOwner: projCtx.ProjectRef.Owner,
			Repo:      projCtx.ProjectRef.Repo,
		}
	}

	// get the new patch document with the patched configuration
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshal the patch's project config so that it is always up to date with the configuration file in the project
	project := &model.Project{}
	if err := model.LoadProjectInto([]byte(projCtx.Patch.PatchedConfig), projCtx.Patch.Project, project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error unmarshaling project config"))
	}

	// retrieve tasks and variant mappings' names
	variantMappings := make(map[string]model.BuildVariant)
	for _, variant := range project.BuildVariants {
		tasksForVariant := []model.BuildVariantTaskUnit{}
		for _, TaskFromVariant := range variant.Tasks {
			if TaskFromVariant.IsGroup {
				tasksForVariant = append(tasksForVariant, model.CreateTasksFromGroup(TaskFromVariant, project)...)
			} else {
				tasksForVariant = append(tasksForVariant, TaskFromVariant)
			}
		}
		variant.Tasks = tasksForVariant
		variantMappings[variant.Name] = variant
	}

	tasksList := []interface{}{}
	for _, task := range project.Tasks {
		// add a task name to the list if it's patchable
		if !(task.Patchable != nil && !*task.Patchable) {
			tasksList = append(tasksList, struct{ Name string }{task.Name})
		}
	}

	commitQueuePosition := 0
	if projCtx.Patch.Alias == evergreen.CommitQueueAlias {
		cq, err := commitqueue.FindOneId(project.Identifier)
		// still display patch page if problem finding commit queue
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error finding commit queue"))
		}
		if cq != nil {
			commitQueuePosition = cq.FindItem(projCtx.Patch.Id.Hex())
		}
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Version             *uiVersion
		Variants            map[string]model.BuildVariant
		Tasks               []interface{}
		CanEdit             bool
		CommitQueuePosition int
		ViewData
	}{versionAsUI, variantMappings, tasksList, currentUser != nil,
		commitQueuePosition, uis.GetCommonViewData(w, r, true, true)},
		"base", "patch_version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulePatch(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	curUser := gimlet.GetUser(r.Context())
	if curUser == nil {
		http.Error(w, "Not authorized to schedule patch", http.StatusUnauthorized)
		return
	}
	// grab patch again, as the diff  was excluded
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshal the project config and set it in the project context
	project := &model.Project{}
	if err = model.LoadProjectInto([]byte(projCtx.Patch.PatchedConfig), projCtx.Patch.Project, project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("Error unmarshaling project config: %v", err))
	}

	patchUpdateReq := patchVariantsTasksRequest{}
	if err = util.ReadJSONInto(util.NewRequestReader(r), &patchUpdateReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	grip.InfoWhen(len(patchUpdateReq.Tasks) > 0 || len(patchUpdateReq.Variants) > 0, message.Fields{
		"source":     "ui_update_patch",
		"message":    "legacy structure is being used",
		"update_req": patchUpdateReq,
		"patch_id":   projCtx.Patch.Id.Hex(),
		"version":    projCtx.Patch.Version,
	})

	tasks := model.TaskVariantPairs{}
	if len(patchUpdateReq.VariantsTasks) > 0 {
		tasks = model.VariantTasksToTVPairs(patchUpdateReq.VariantsTasks)
	} else {
		for _, v := range patchUpdateReq.Variants {
			for _, t := range patchUpdateReq.Tasks {
				if project.FindTaskForVariant(t, v) != nil {
					tasks.ExecTasks = append(tasks.ExecTasks, model.TVPair{Variant: v, TaskName: t})
				}
			}
		}
	}

	tasks.ExecTasks = model.IncludePatchDependencies(project, tasks.ExecTasks)

	if err = model.ValidateTVPairs(project, tasks.ExecTasks); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// update the description for both reconfigured and new patches
	if err = projCtx.Patch.SetDescription(patchUpdateReq.Description); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "Error setting description"))
		return
	}

	// update the description for both reconfigured and new patches
	if err = projCtx.Patch.SetVariantsTasks(tasks.TVPairsToVariantTasks()); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "Error setting description"))
		return
	}

	if projCtx.Patch.Version != "" {
		projCtx.Patch.Activated = true
		// This patch has already been finalized, just add the new builds and tasks
		if projCtx.Version == nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Errorf("Couldn't find patch for id %v", projCtx.Patch.Version))
			return
		}

		// First add new tasks to existing builds, if necessary
		err = model.AddNewTasksForPatch(context.Background(), projCtx.Patch, projCtx.Version, project, tasks)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error creating new tasks for version `%s`", projCtx.Version.Id))
			return
		}

		err := model.AddNewBuildsForPatch(projCtx.Patch, projCtx.Version, project, tasks)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error creating new builds for version `%s`", projCtx.Version.Id))
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Builds and tasks successfully added to patch."))
		gimlet.WriteJSON(w, struct {
			VersionId string `json:"version"`
		}{projCtx.Version.Id})

	} else {
		githubOauthToken, err := uis.Settings.GetGithubOauthToken()
		if err != nil {
			gimlet.WriteJSONError(w, err)
			return
		}
		projCtx.Patch.Activated = true
		err = projCtx.Patch.SetVariantsTasks(tasks.TVPairsToVariantTasks())
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error setting patch variants and tasks"))
			return
		}

		requester := evergreen.PatchVersionRequester
		if projCtx.Patch.IsGithubPRPatch() {
			requester = evergreen.GithubPRRequester
		}
		if projCtx.Patch.IsPRMergePatch() {
			requester = evergreen.MergeTestRequester
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		ver, err := model.FinalizePatch(ctx, projCtx.Patch, requester, githubOauthToken)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error finalizing patch"))
			return
		}

		if projCtx.Patch.IsGithubPRPatch() {
			job := units.NewGithubStatusUpdateJobForNewPatch(projCtx.Patch.Id.Hex())
			if err := uis.queue.Put(ctx, job); err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError,
					errors.Wrap(err, "Error adding github status update job to queue"))
				return
			}
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Patch builds are scheduled."))
		gimlet.WriteJSON(w, struct {
			VersionId string `json:"version"`
		}{ver.Id})
	}
}

func (uis *UIServer) diffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	// We have to reload the patch outside of the project context,
	// since the raw diff is excluded by default. This redundancy is
	// worth the time savings this behavior offers other pages.
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("finding patch files: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	uis.render.WriteResponse(w, http.StatusOK, fullPatch, "base", "diff.html")
}

func (uis *UIServer) fileDiffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("error finding patch: %s", err.Error()),
			http.StatusInternalServerError)
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data        patch.Patch
		FileName    string
		PatchNumber string
	}{*fullPatch, r.FormValue("file_name"), r.FormValue("patch_number")},
		"base", "file_diff.html")
}

func (uis *UIServer) rawDiffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("error fetching patch files: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	patchNum, err := strconv.Atoi(r.FormValue("patch_number"))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting patch number: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if patchNum < 0 || patchNum >= len(fullPatch.Patches) {
		http.Error(w, "patch number out of range", http.StatusInternalServerError)
		return
	}
	diff := fullPatch.Patches[patchNum].PatchSet.Patch
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(diff))
	grip.Warning(err)
}
