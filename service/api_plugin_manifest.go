package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// manifestLoadHandler attempts to get the manifest, if it exists it updates the expansions and returns
// If it does not exist it performs GitHub API calls for each of the project's modules and gets
// the head revision of the branch and inserts it into the manifest collection.
// If there is a duplicate key error, then do a find on the manifest again.
func (as *APIServer) manifestLoadHandler(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	projectRef, err := model.FindOneProjectRef(task.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Wrapf(err, "projectRef not found for project %s", task.Project))
		return
	}

	project, err := model.FindProject("", projectRef)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Wrapf(err, "project not found for ProjectRef %s", projectRef.Identifier))
		return
	}
	if project == nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Errorf("empty project not found for ProjectRef %s", projectRef.Identifier))
		return
	}

	// try to get the manifest
	v, err := version.FindOne(version.ById(task.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error retrieving version for task"))
		return
	}
	if v == nil {
		as.LoggedError(w, r, http.StatusNotFound, errors.Errorf("version not found: %s", task.Version))
		return
	}
	currentManifest, err := manifest.FindOne(manifest.ByProjectAndRevision(v.Identifier, v.Revision))
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Wrapf(err, "error retrieving manifest with version id %s", task.Version))
		return
	}
	if currentManifest != nil {
		gimlet.WriteJSON(w, currentManifest)
		return
	}

	if task.Version == "" {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Errorf("found empty version when retrieving manifest for %s", projectRef.Identifier))
		return
	}

	// attempt to insert a manifest after making GitHub API calls
	manifest, err := repotracker.CreateManifest(*v, project, projectRef.Branch, &as.Settings)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error storing new manifest"))
		return
	}

	gimlet.WriteJSON(w, manifest)
}
