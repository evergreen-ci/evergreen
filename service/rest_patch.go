package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
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
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}

	err := projCtx.Patch.FetchPatchFiles(true)
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

	gimlet.WriteJSON(w, destPatch)
}

// getPatchConfig returns the patched config for a given patch.
func (restapi restAPI) getPatchConfig(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	if projCtx.Patch == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "patch not found"})
		return
	}

	settings := restapi.GetSettings()
	var pp *model.ParserProject
	var err error
	if projCtx.Patch.ProjectStorageMethod != "" {
		pp, err = model.ParserProjectFindOneByID(r.Context(), &settings, projCtx.Patch.ProjectStorageMethod, projCtx.Patch.Id.Hex())
		if err != nil {
			gimlet.WriteJSONInternalError(w, errors.Wrapf(err, "finding parser project for patch '%s'", projCtx.Patch.Id.Hex()))
			return
		}
		if pp == nil {
			gimlet.WriteJSONInternalError(w, fmt.Sprintf("parser project for patch '%s' not found", projCtx.Patch.Id.Hex()))
			return
		}
	} else if projCtx.Version != nil {
		pp, err = model.ParserProjectFindOneByID(r.Context(), &settings, projCtx.Version.ProjectStorageMethod, projCtx.Version.Id)
		if err != nil {
			gimlet.WriteJSONInternalError(w, errors.Wrapf(err, "finding parser project '%s' for version '%s'", projCtx.Version.Id, projCtx.Version.Id))
			return
		}
		if pp == nil {
			gimlet.WriteJSONInternalError(w, fmt.Sprintf("parser project '%s' for patch '%s' not found", projCtx.Version.Id, projCtx.Patch.Id.Hex()))
			return
		}
	} else {
		gimlet.WriteJSONInternalError(w, fmt.Sprintf("cannot get parser project for patch '%s' because patch has no associated parser project and version is nil", projCtx.Patch.Id.Hex()))
		return
	}

	projBytes, err := yaml.Marshal(pp)
	if err != nil {
		gimlet.WriteJSONInternalError(w, errors.Wrapf(err, "marshalling parser project '%s' to YAML", pp.Id))
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(projBytes)
	grip.Warning(message.WrapError(err, message.Fields{
		"message":  "could not write parser project to response",
		"patch_id": projCtx.Patch.Id.Hex(),
		"route":    "/rest/v1/patches/{patch_id}/config",
	}))
}
