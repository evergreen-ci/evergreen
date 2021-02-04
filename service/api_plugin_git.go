package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (as *APIServer) gitServePatch(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	patch, err := patch.FindOne(patch.ByVersion(task.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "problem fetching patch for task '%s' from db", task.Id))
		return
	}
	if patch == nil {
		as.LoggedError(w, r, http.StatusNotFound,
			errors.Errorf("no patch found for task %s", task.Id))
		return
	}
	gimlet.WriteJSON(w, patch)
}

func (as *APIServer) gitServePatchFile(w http.ResponseWriter, r *http.Request) {
	fileId := gimlet.GetVars(r)["patchfile_id"]
	patchContents, err := patch.FetchPatchContents(fileId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file from db: %v", err), http.StatusInternalServerError)
		return
	}
	gimlet.WriteText(w, patchContents)
}
