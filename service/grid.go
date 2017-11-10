package service

import (
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/grid"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	// how many prior versions to fetch by default
	defaultGridDepth = 20
)

func (uis *UIServer) grid(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	// If no version was specified in the URL, grab the latest version on the project
	if projCtx.Version == nil {
		var v []version.Version
		v, err = version.Find(version.ByMostRecentForRequester(project.Identifier, evergreen.RepotrackerVersionRequester).Limit(1))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error finding version"))
			return
		}
		if len(v) > 0 {
			projCtx.Version = &v[0]
		}
	}

	var versions map[string]version.Version
	var cells grid.Grid
	var failures grid.Failures
	var revisionFailures grid.RevisionFailures
	var depth int

	d := mux.Vars(r)["depth"]
	if d == "" {
		depth = defaultGridDepth
	} else {
		depth, err = strconv.Atoi(d)
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "Error converting depth"))
			return
		}
		if depth < 0 {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Depth must be non-negative, got %v", depth))
			return
		}
	}

	if projCtx.Version != nil {
		recentVersions, err := version.Find(version.
			ByProjectIdAndOrder(projCtx.Version.Identifier, projCtx.Version.RevisionOrderNumber).
			WithFields(version.IdKey, version.RevisionKey, version.RevisionOrderNumberKey, version.MessageKey, version.AuthorKey, version.CreateTimeKey).
			Sort([]string{"-" + version.RevisionOrderNumberKey}).
			Limit(depth + 1))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching versions"))
			return
		}

		versions = make(map[string]version.Version, len(recentVersions))
		for _, v := range recentVersions {
			versions[v.Revision] = v
		}

		cells, err = grid.FetchCells(*projCtx.Version, depth)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching builds"))
			return
		}

		failures, err = grid.FetchFailures(*projCtx.Version, depth)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching builds"))
			return
		}

		revisionFailures, err = grid.FetchRevisionOrderFailures(*projCtx.Version, depth)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching revision failures"))
			return
		}
	} else {
		versions = make(map[string]version.Version)
		cells = make(grid.Grid, 0)
		failures = make(grid.Failures, 0)
		revisionFailures = make(grid.RevisionFailures, 0)
	}
	uis.WriteHTML(w, http.StatusOK, struct {
		Versions         map[string]version.Version
		GridCells        grid.Grid
		Failures         grid.Failures
		RevisionFailures grid.RevisionFailures
		ViewData
	}{versions, cells, failures, revisionFailures, uis.GetCommonViewData(w, r, false, true)}, "base", "grid.html", "base_angular.html", "menu.html")
}
