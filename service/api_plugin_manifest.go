package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type legacyService interface {
	LoggedError(http.ResponseWriter, *http.Request, int, error)
	WriteJSON(http.ResponseWriter, int, interface{})
}

func makeGetManifestHandler(s legacyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := mux.Vars(r)["project_id"]
		revision := mux.Vars(r)["revision"]

		version, err := version.FindOne(version.ByProjectIdAndRevision(project, revision))
		if err != nil {
			s.LoggedError(w, r, http.StatusBadRequest, errors.Wrapf(err,
				"problem getting version for project %s with revision %s",
				project, revision))
			return
		}

		if version == nil {
			s.LoggedError(w, r, http.StatusNotFound, errors.Errorf(
				"version not found for project %s, with revision %s",
				project, revision))
			return
		}

		foundManifest, err := manifest.FindOne(manifest.ById(version.Id))
		if err != nil {
			s.LoggedError(w, r, http.StatusBadRequest, errors.Wrapf(err,
				"problem getting manifest with version id %s", version.Id))
			return
		}

		if foundManifest == nil {
			s.LoggedError(w, r, http.StatusNotFound,
				errors.Errorf("manifest not found for version %s", version.Id))
			return
		}

		s.WriteJSON(w, http.StatusOK, foundManifest)
		return
	}
}
