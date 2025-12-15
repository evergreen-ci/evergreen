package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
)

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
