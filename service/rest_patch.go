package service

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
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
	if projCtx.Patch == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}

	err := projCtx.Patch.FetchPatchFiles()
	if err != nil {
		restapi.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrap(err, "error occurred fetching patch data"))
		return
	}

	destPatch := &RestPatch{
		Id:          projCtx.Patch.Id.Hex(),
		Description: projCtx.Patch.Description,
		Project:     projCtx.Patch.Project,
		Revision:    projCtx.Patch.Githash,
		PatchNumber: projCtx.Patch.PatchNumber,
		Author:      projCtx.Patch.Author,
		Version:     projCtx.Patch.Version,
		CreateTime:  projCtx.Patch.CreateTime,
		Patches:     projCtx.Patch.Patches,
	}

	restapi.WriteJSON(w, http.StatusOK, destPatch)
	return
}

// getPatchConfig returns the patched config for a given patch.
func (restapi restAPI) getPatchConfig(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	if projCtx.Patch == nil {
		restapi.WriteJSON(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(projCtx.Patch.PatchedConfig))
}
