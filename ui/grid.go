package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/grid"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
)

var (
	// how many prior versions to fetch by default
	defaultGridDepth = 20
)

func (uis *UIServer) grid(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	// If no version was specified in the URL, grab the latest version on the project
	if projCtx.Version == nil {
		v, err := version.Find(version.ByMostRecentForRequester(projCtx.Project.Identifier, evergreen.RepotrackerVersionRequester).Limit(1))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding version: %v", err), http.StatusInternalServerError)
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
	var err error

	d := mux.Vars(r)["depth"]
	if d == "" {
		depth = defaultGridDepth
	} else {
		depth, err = strconv.Atoi(d)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error converting depth: %v", err), http.StatusBadRequest)
			return
		}
		if depth < 0 {
			http.Error(w, fmt.Sprintf("Depth must be non-negative, got %v", depth), http.StatusBadRequest)
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
			http.Error(w, fmt.Sprintf("Error fetching versions: %v", err), http.StatusInternalServerError)
			return
		}

		versions = make(map[string]version.Version, len(recentVersions))
		for _, v := range recentVersions {
			versions[v.Revision] = v
		}

		cells, err = grid.FetchCells(*projCtx.Version, depth)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}

		failures, err = grid.FetchFailures(*projCtx.Version, depth)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}

		revisionFailures, err = grid.FetchRevisionOrderFailures(*projCtx.Version, depth)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching revision failures: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		versions = make(map[string]version.Version)
		cells = make(grid.Grid, 0)
		failures = make(grid.Failures, 0)
		revisionFailures = make(grid.RevisionFailures, 0)
	}
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData      projectContext
		Versions         map[string]version.Version
		GridCells        grid.Grid
		Failures         grid.Failures
		RevisionFailures grid.RevisionFailures
		User             *user.DBUser
	}{projCtx, versions, cells, failures, revisionFailures, GetUser(r)}, "base", "grid.html", "base_angular.html", "menu.html")
}
