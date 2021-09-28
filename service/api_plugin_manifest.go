package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// manifestLoadHandler attempts to get the manifest, if it exists it updates the expansions and returns
// If it does not exist it performs GitHub API calls for each of the project's modules and gets
// the head revision of the branch and inserts it into the manifest collection.
// If there is a duplicate key error, then do a find on the manifest again.
func (as *APIServer) manifestLoadHandler(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	projectRef, err := model.FindMergedProjectRef(task.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "error finding project '%s'", task.Project))
		return
	}
	if projectRef == nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Errorf("project ref '%s' doesn't exist", task.Project))
	}

	v, err := model.VersionFindOne(model.VersionById(task.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error retrieving version for task"))
		return
	}
	if v == nil {
		as.LoggedError(w, r, http.StatusNotFound, errors.Errorf("version not found: %s", task.Version))
		return
	}
	currentManifest, err := manifest.FindFromVersion(v.Id, v.Identifier, v.Revision, v.Requester)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Wrapf(err, "error retrieving manifest with version id %s", task.Version))
		return
	}

	var project *model.Project
	project, _, err = model.LoadProjectForVersion(v, v.Identifier, false)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "error loading project from version"))
		return
	}

	if currentManifest != nil && project.Modules.IsIdentical(*currentManifest) {
		gimlet.WriteJSON(w, currentManifest)
		return
	}

	// attempt to insert a manifest after making GitHub API calls
	manifest, err := repotracker.CreateManifest(*v, project, projectRef, &as.Settings)
	if err != nil {
		if apiErr, ok := errors.Cause(err).(thirdparty.APIRequestError); ok && apiErr.StatusCode == http.StatusNotFound {
			as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "manifest resource not found"))
			return
		}
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error storing new manifest"))
		return
	}

	gimlet.WriteJSON(w, manifest)
}
