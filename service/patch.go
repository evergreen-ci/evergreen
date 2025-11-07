package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
)

func (uis *UIServer) diffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	// We have to reload the patch outside of the project context,
	// since the raw diff is excluded by default. This redundancy is
	// worth the time savings this behavior offers other pages.
	fullPatch, err := patch.FindOne(r.Context(), patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if fullPatch == nil {
		http.Error(w, fmt.Sprintf("could not find patch '%s'", projCtx.Patch.Id),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(r.Context()); err != nil {
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
	fullPatch, err := patch.FindOne(r.Context(), patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if fullPatch == nil {
		http.Error(w, fmt.Sprintf("could not find patch '%s'", projCtx.Patch.Id),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(r.Context()); err != nil {
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
	fullPatch, err := patch.FindOne(r.Context(), patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %s", err.Error()),
			http.StatusInternalServerError)
		return
	}
	if fullPatch == nil {
		http.Error(w, fmt.Sprintf("could not find patch '%s'", projCtx.Patch.Id),
			http.StatusInternalServerError)
		return
	}
	if err = fullPatch.FetchPatchFiles(r.Context()); err != nil {
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
