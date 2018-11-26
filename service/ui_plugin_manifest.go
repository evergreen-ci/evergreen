package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/gimlet"
)

func (uis *UIServer) GetManifest(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	project := vars["project_id"]
	revision := vars["revision"]

	version, err := version.FindOne(version.ByProjectIdAndRevision(project, revision))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting version for project %v with revision %v: %v",
			project, revision, err), http.StatusBadRequest)
		return
	}
	if version == nil {
		http.Error(w, fmt.Sprintf("version not found for project %v, with revision %v", project, revision),
			http.StatusNotFound)
		return
	}

	foundManifest, err := manifest.FindFromVersion(version.Id, version.Identifier, version.Revision)
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting manifest with version id %v: %v",
			version.Id, err), http.StatusBadRequest)
		return
	}
	if foundManifest == nil {
		http.Error(w, fmt.Sprintf("manifest not found for version %v", version.Id), http.StatusNotFound)
		return
	}
	gimlet.WriteJSON(w, foundManifest)
}
