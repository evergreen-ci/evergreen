package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"net/http"
)

func (uis *UIServer) patchPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Patch == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var versionAsUI *uiVersion
	if projCtx.Version != nil { // Patch is already finalized
		versionAsUI = &uiVersion{
			Version:   *projCtx.Version,
			RepoOwner: projCtx.ProjectRef.Owner,
			Repo:      projCtx.ProjectRef.Repo,
		}
	}

	variantMappings := make(map[string]model.BuildVariant)
	for _, variant := range projCtx.Project.BuildVariants {
		variantMappings[variant.Name] = variant
	}

	tasksList := []interface{}{}
	for _, task := range projCtx.Project.Tasks {
		tasksList = append(tasksList,
			struct {
				Name string
			}{task.Name})
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Version     *uiVersion
		Variants    map[string]model.BuildVariant
		Tasks       []interface{}
	}{projCtx, GetUser(r), versionAsUI, variantMappings, tasksList}, "base",
		"patch_new.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulePatch(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}

	// grab patch again, as the diff  was excluded
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	patchUpdateReq := struct {
		Variants    []string `json:"variants"`
		Tasks       []string `json:"tasks"`
		Description string   `json:"description"`
	}{}

	err = util.ReadJSONInto(r.Body, &patchUpdateReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if projCtx.Patch.Version != "" {
		// This patch has already been finalized, just add the new builds and tasks
		if projCtx.Version == nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Couldn't find patch for id %v", projCtx.Patch.Version))
			return
		}

		// First add new tasks to existing builds, if necessary
		if len(patchUpdateReq.Tasks) > 0 {
			err = model.AddNewTasksForPatch(projCtx.Patch, projCtx.Version, patchUpdateReq.Tasks)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError,
					fmt.Errorf("Error creating new tasks: `%v` for version `%v`", err, projCtx.Version.Id))
				return
			}
		}

		if len(patchUpdateReq.Variants) > 0 {
			_, err := model.AddNewBuildsForPatch(projCtx.Patch, projCtx.Version, patchUpdateReq.Variants)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError,
					fmt.Errorf("Error creating new builds: `%v` for version `%v`", err, projCtx.Version.Id))
				return
			}
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Builds and tasks successfully added to patch."))
		uis.WriteJSON(w, http.StatusOK, struct {
			VersionId string `json:"version"`
		}{projCtx.Version.Id})
	} else {
		err = projCtx.Patch.SetVariantsAndTasks(patchUpdateReq.Variants, patchUpdateReq.Tasks)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Error setting patch variants and tasks: %v", err))
			return
		}

		if err = projCtx.Patch.SetDescription(patchUpdateReq.Description); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Error setting description: %v", err))
			return
		}
		ver, err := validator.ValidateAndFinalize(projCtx.Patch, &uis.Settings)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error finalizing patch: %v", err))
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
	uis.WriteHTML(w, http.StatusOK, struct {
		Data        patch.Patch
		FileName    string
		PatchNumber string
	}{*fullPatch, r.FormValue("file_name"), r.FormValue("patch_number")},
		"base", "file_diff.html")
}
