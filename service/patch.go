package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
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
	if err := yaml.Unmarshal([]byte(projCtx.Patch.PatchedConfig), project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error unmarshaling project config"))
	}
	projCtx.Project = project

	// retrieve tasks and variant mappings' names
	variantMappings := make(map[string]model.BuildVariant)
	for _, variant := range projCtx.Project.BuildVariants {
		variantMappings[variant.Name] = variant
	}

	tasksList := []interface{}{}
	for _, task := range projCtx.Project.Tasks {
		// add a task name to the list if it's patchable
		if !(task.Patchable != nil && !*task.Patchable) {
			tasksList = append(tasksList, struct{ Name string }{task.Name})
		}
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		Version  *uiVersion
		Variants map[string]model.BuildVariant
		Tasks    []interface{}
		CanEdit  bool
		ViewData
	}{versionAsUI, variantMappings, tasksList, uis.canEditPatch(currentUser, projCtx.Patch), uis.GetCommonViewData(w, r, true, true)}, "base",
		"patch_version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulePatch(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	curUser := GetUser(r)
	if !uis.canEditPatch(curUser, projCtx.Patch) {
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
	if err = yaml.Unmarshal([]byte(projCtx.Patch.PatchedConfig), project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("Error unmarshaling project config: %v", err))
	}
	projCtx.Project = project

	patchUpdateReq := patchVariantsTasksRequest{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), &patchUpdateReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pairs []model.TVPair
	if len(patchUpdateReq.VariantsTasks) > 0 {
		pairs = model.VariantTasksToTVPairs(patchUpdateReq.VariantsTasks)
	} else {
		for _, v := range patchUpdateReq.Variants {
			for _, t := range patchUpdateReq.Tasks {
				if project.FindTaskForVariant(t, v) != nil {
					pairs = append(pairs, model.TVPair{v, t})
				}
			}
		}
	}

	pairs = model.IncludePatchDependencies(projCtx.Project, pairs)

	if err = model.ValidateTVPairs(projCtx.Project, pairs); err != nil {
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
	if err = projCtx.Patch.SetVariantsTasks(model.TVPairsToVariantTasks(pairs)); err != nil {
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
		err = model.AddNewTasksForPatch(projCtx.Patch, projCtx.Version, projCtx.Project, pairs)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error creating new tasks for version `%v`", projCtx.Version.Id))
			return
		}

		err := model.AddNewBuildsForPatch(projCtx.Patch, projCtx.Version, projCtx.Project, pairs)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error creating new builds for version `%v`", err, projCtx.Version.Id))
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Builds and tasks successfully added to patch."))
		uis.WriteJSON(w, http.StatusOK, struct {
			VersionId string `json:"version"`
		}{projCtx.Version.Id})
	} else {
		projCtx.Patch.Activated = true
		err = projCtx.Patch.SetVariantsTasks(model.TVPairsToVariantTasks(pairs))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error setting patch variants and tasks"))
			return
		}

		ver, err := model.FinalizePatch(projCtx.Patch, &uis.Settings)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error finalizing patch"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Patch builds are scheduled."))
		uis.WriteJSON(w, http.StatusOK, struct {
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
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("finding patch files: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	uis.WriteHTML(w, http.StatusOK, fullPatch, "base", "diff.html")
}

func (uis *UIServer) fileDiffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("error finding patch: %v", err.Error),
			http.StatusInternalServerError)
	}
	uis.WriteHTML(w, http.StatusOK, struct {
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
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(); err != nil {
		http.Error(w, fmt.Sprintf("error fetching patch files: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	patchNum, err := strconv.Atoi(r.FormValue("patch_number"))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting patch number: %v", err.Error),
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
