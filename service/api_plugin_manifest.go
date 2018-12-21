package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
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

	v, err := model.VersionFindOne(model.VersionById(task.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error retrieving version for task"))
		return
	}
	if v == nil {
		as.LoggedError(w, r, http.StatusNotFound, errors.Errorf("version not found: %s", task.Version))
		return
	}
	currentManifest, err := manifest.FindFromVersion(v.Id, v.Identifier, v.Revision)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest,
			errors.Wrapf(err, "error retrieving manifest with version id %s", task.Version))
		return
	}

	project := &model.Project{}
	err = model.LoadProjectInto([]byte(v.Config), v.Identifier, project)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "error loading project from version"))
		return
	}

	if currentManifest != nil && project.Modules.IsIdentical(*currentManifest) {
		gimlet.WriteJSON(w, currentManifest)
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
