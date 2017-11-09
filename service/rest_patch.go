package service

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type RestPatch struct {
	Id          string              `json:"_id"`
	Description string              `json:"desc"`
	Project     string              `json:"project"`
	Revision    string              `json:"revision"`
	PatchNumber int                 `json:"patch_number"`
	Author      string              `json:"author"`
	Version     string              `json:"version"`
	CreateTime  time.Time           `json:"create_time"`
	Patches     []patch.ModulePatch `json:"patches"`
}

// Returns a JSON response with the marshaled output of the task
// specified in the request.
func (restapi restAPI) getPatch(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	patchDoc, _ := projCtx.GetPatch()
	if patchDoc == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}

	if err := patchDoc.FetchPatchFiles(); err != nil {
		restapi.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "error occurred fetching patch data"))
		return
	}

	destPatch := &RestPatch{
		Id:          patchDoc.Id.Hex(),
		Description: patchDoc.Description,
		Project:     patchDoc.Project,
		Revision:    patchDoc.Githash,
		PatchNumber: patchDoc.PatchNumber,
		Author:      patchDoc.Author,
		Version:     patchDoc.Version,
		CreateTime:  patchDoc.CreateTime,
		Patches:     patchDoc.Patches,
	}

	restapi.WriteJSON(w, http.StatusOK, destPatch)
}

// getPatchConfig returns the patched config for a given patch.
func (restapi restAPI) getPatchConfig(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	patchDoc, _ := projCtx.GetPatch()
	if patchDoc == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(patchDoc.PatchedConfig))
	grip.Warning(err)
}
