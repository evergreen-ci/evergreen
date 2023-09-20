package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/units"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func (uis *UIServer) patchPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	currentUser := MustHaveUser(r)
	spruceLink := fmt.Sprintf("%s/patch/%s/configure", uis.Settings.Ui.UIv2Url, projCtx.Patch.Id.Hex())
	if RedirectSpruceUsers(w, r, spruceLink) {
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

	// get the new patch document with the patched configuration
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshall project and get project variants and tasks
	variantsAndTasksFromProject, err := model.GetVariantsAndTasksFromPatchProject(r.Context(), uis.env.Settings(), projCtx.Patch)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	commitQueuePosition := 0
	if projCtx.Patch.IsCommitQueuePatch() {
		cq, err := commitqueue.FindOneId(projCtx.ProjectRef.Id)
		// still display patch page if problem finding commit queue
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error finding commit queue"))
		}
		if cq != nil {
			commitQueuePosition = cq.FindItem(projCtx.Patch.Id.Hex())
		}
	}

	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = spruceLink
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Version             *uiVersion
		Variants            map[string]model.BuildVariant
		Tasks               []struct{ Name string }
		CanEdit             bool
		CommitQueuePosition int
		NewUILink           string
		ViewData
	}{versionAsUI, variantsAndTasksFromProject.Variants, variantsAndTasksFromProject.Tasks, currentUser != nil,
		commitQueuePosition, newUILink, uis.GetCommonViewData(w, r, true, true)},
		"base", "patch_version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulePatchUI(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New("patch not found"))
	}
	curUser := gimlet.GetUser(r.Context())
	if curUser == nil {
		uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("Not authorized to schedule patch"))
	}
	patchUpdateReq := model.PatchUpdate{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), &patchUpdateReq); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
	}
	patchUpdateReq.Caller = curUser.Username()

	status, err := units.SchedulePatch(r.Context(), uis.env, projCtx.Patch.Id.Hex(), projCtx.Version, patchUpdateReq)
	if err != nil {
		uis.LoggedError(w, r, status, err)
		return
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Patch successfully configured."))
	gimlet.WriteJSON(w, struct {
		VersionId string `json:"version"`
	}{projCtx.Patch.Id.Hex()})

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
	if err = fullPatch.FetchPatchFiles(false); err != nil {
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
	if err = fullPatch.FetchPatchFiles(false); err != nil {
		http.Error(w, fmt.Sprintf("error finding patch: %s", err.Error()),
			http.StatusInternalServerError)
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data         patch.Patch
		FileName     string
		PatchNumber  string
		CommitNumber string
	}{*fullPatch, r.FormValue("file_name"), r.FormValue("patch_number"), r.FormValue("commit_number")},
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
	if err = fullPatch.FetchPatchFiles(true); err != nil {
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
