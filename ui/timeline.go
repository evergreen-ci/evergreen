package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strconv"
)

func (uis *UIServer) timelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	skip, perPage := getSkipAndLimit(r, DefaultSkip, DefaultLimit)
	data, err := getTimelineData(projCtx.Project.Identifier, evergreen.RepotrackerVersionRequester, skip, perPage)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting timeline data: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	uis.WriteJSON(w, http.StatusOK, data)
}

func (uis *UIServer) timeline(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
	}{projCtx, GetUser(r)}, "base", "timeline.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) patchTimeline(w http.ResponseWriter, r *http.Request) {
	uis.patchTimelineWrapper("", w, r)
}

func (uis *UIServer) myPatchesTimeline(w http.ResponseWriter, r *http.Request) {
	author := MustHaveUser(r).Username()
	uis.patchTimelineWrapper(author, w, r)
}

func (uis *UIServer) userPatchesTimeline(w http.ResponseWriter, r *http.Request) {
	author := mux.Vars(r)["user_id"]
	uis.patchTimelineWrapper(author, w, r)
}

func (uis *UIServer) patchTimelineWrapper(author string, w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Author      string
	}{projCtx, GetUser(r), author}, "base", "patches.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) userPatchTimeline(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	author := mux.Vars(r)["user_id"]
	if len(author) == 0 {
		author = GetUser(r).Username()
	}
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Author      string
	}{projCtx, GetUser(r), author}, "base", "patches.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) patchTimelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	pageNum, err := strconv.Atoi(r.FormValue("page"))
	if err != nil {
		pageNum = 0
	}
	skip := pageNum * DefaultLimit

	user := mux.Vars(r)["user_id"]
	var patches []patch.Patch
	if len(user) > 0 {
		patches, err = patch.Find(patch.ByUser(user).
			Project(patch.ExcludePatchDiff).
			Sort([]string{"-" + patch.CreateTimeKey}).
			Skip(skip).Limit(DefaultLimit))
	} else {
		patches, err = patch.Find(patch.ByProject(projCtx.Project.Identifier).
			Sort([]string{"-" + patch.CreateTimeKey}).
			Project(patch.ExcludePatchDiff).
			Skip(skip).Limit(DefaultLimit))
	}
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error fetching patches for %v: %v", projCtx.Project.Identifier, err))
		return
	}

	versionIds := make([]string, 0, len(patches))
	uiPatches := make([]uiPatch, 0, len(patches))
	for _, patch := range patches {
		if patch.Version != "" {
			versionIds = append(versionIds, patch.Version)
		}
		baseVersion, err := version.FindOne(version.ByProjectIdAndRevision(patch.Project, patch.Githash))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		var baseVersionId string
		if baseVersion != nil {
			baseVersionId = baseVersion.Id
		}
		patch.Patches = nil
		uiPatches = append(uiPatches, uiPatch{Patch: patch, BaseVersionId: baseVersionId})
	}
	versions, err := version.Find(version.ByIds(versionIds).WithoutFields(version.ConfigKey))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error fetching versions for patches: %v", err))
		return
	}

	versionsMap := map[string]*uiVersion{}
	for _, version := range versions {
		versionUI, err := PopulateUIVersion(&version)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		versionsMap[version.Id] = versionUI
	}

	data := struct {
		VersionsMap map[string]*uiVersion
		UIPatches   []uiPatch
		PageNum     int
	}{versionsMap, uiPatches, pageNum}

	uis.WriteJSON(w, http.StatusOK, data)
}

func (uis *UIServer) buildmaster(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
	}

	// If no version was specified in the URL, grab the latest version on the project
	if projCtx.Version == nil {
		versions, err := version.Find(version.ByMostRecentForRequester(projCtx.Project.Identifier, evergreen.RepotrackerVersionRequester).Limit(1))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding version: %v", err), http.StatusInternalServerError)
			return
		}
		if len(versions) > 0 {
			projCtx.Version = &versions[0]
		}
	}

	var recentVersions []version.Version
	var gitspecMap map[string]version.Version
	var builds []build.Build
	var buildmasterData []bson.M
	var err error

	if projCtx.Version != nil {
		recentVersions, err = version.Find(
			version.ByProjectId(projCtx.Version.Project).
				WithFields(version.IdKey, version.RevisionKey, version.RevisionOrderNumberKey, version.MessageKey).
				Sort([]string{"-" + version.RevisionOrderNumberKey}).
				Limit(50))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching versions: %v", err), http.StatusInternalServerError)
			return
		}
		gitspecMap = make(map[string]version.Version)
		for _, ver := range recentVersions {
			gitspecMap[ver.Revision] = ver
		}

		//TODO make this return a non-bson map
		buildmasterData, err = model.GetBuildmasterData(*projCtx.Version, 500)

		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}

		builds, err = build.Find(build.ByIds(projCtx.Version.BuildIds))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		recentVersions = make([]version.Version, 0)
		gitspecMap = make(map[string]version.Version)
		builds = make([]build.Build, 0)
		buildmasterData = make([]bson.M, 0)
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData       projectContext
		VersionsByGitspec map[string]version.Version
		Builds            []build.Build
		VersionHistory    []bson.M
		User              *user.DBUser
	}{projCtx, gitspecMap, builds, buildmasterData, GetUser(r)}, "base", "buildmaster.html", "base_angular.html", "menu.html")
}
