package service

import (
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/gorilla/mux"
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
	as.WriteJSON(w, http.StatusOK, patch)

}
func (as *APIServer) gitServePatchFile(w http.ResponseWriter, r *http.Request) {
	fileId := mux.Vars(r)["patchfile_id"]
	data, err := db.GetGridFile(patch.GridFSPrefix, fileId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file from db: %v", err), http.StatusInternalServerError)
		return
	}
	defer data.Close()
	_, _ = io.Copy(w, data)
}
