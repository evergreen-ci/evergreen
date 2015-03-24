package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"fmt"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
)

func (uis *UIServer) timelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	skip, perPage := getSkipAndLimit(r, DefaultSkip, DefaultLimit)
	data, err := getTimelineData(projCtx.Project.Identifier, mci.RepotrackerVersionRequester, skip, perPage)
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
		User        *model.DBUser
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
		User        *model.DBUser
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
		User        *model.DBUser
		Author      string
	}{projCtx, GetUser(r), author}, "base", "patches.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) patchTimelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	skip, pageNum := getSkipAndLimit(r, DefaultSkip, DefaultLimit)

	user := mux.Vars(r)["user_id"]
	var patches []model.Patch
	var err error
	if len(user) > 0 {
		patches, err = model.FindPatchesByUser(
			user,
			[]string{fmt.Sprintf("-%v", model.PatchCreateTimeKey)},
			skip,
			DefaultLimit)
	} else {
		patches, err = model.FindAllPatchesByProject(projCtx.Project.Identifier, []string{"-create_time"}, skip, DefaultLimit)
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
		baseVersionId := model.FindBaseVersionIdForRevision(patch.Project, patch.Githash)
		patch.Patches = nil
		uiPatches = append(uiPatches, uiPatch{Patch: patch, BaseVersionId: baseVersionId})
	}
	versions, err := model.FindAllVersions(bson.M{"_id": bson.M{"$in": versionIds}}, bson.M{model.VersionConfigKey: 0}, []string{}, 0, 0)
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
		versions, err := model.FindMostRecentVersions(projCtx.Project.Identifier, "gitter_request", 0, 1)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error finding version: %v", err), http.StatusInternalServerError)
			return
		}
		if len(versions) > 0 {
			projCtx.Version = &versions[0]
		}
	}

	var recentVersions []model.Version
	var gitspecMap map[string]model.Version
	var builds []model.Build
	var buildmasterData []bson.M
	var err error

	if projCtx.Version != nil {
		recentVersions, err = model.FindAllVersions(
			bson.M{
				"r":      "gitter_request",
				"branch": projCtx.Version.Project,
			},
			bson.M{"_id": 1, "gitspec": 1, "order": 1, "message": 1}, []string{"-order"}, 0, 50)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching versions: %v", err), http.StatusInternalServerError)
			return
		}
		gitspecMap = make(map[string]model.Version)
		for _, ver := range recentVersions {
			gitspecMap[ver.Revision] = ver
		}

		buildmasterData, err = model.GetBuildmasterData(*projCtx.Version, 500)

		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}

		builds, err = model.FindAllBuilds(bson.M{"_id": bson.M{"$in": projCtx.Version.BuildIds}}, bson.M{},
			db.NoSort, db.NoSkip, db.NoLimit)

		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching builds: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		recentVersions = make([]model.Version, 0)
		gitspecMap = make(map[string]model.Version)
		builds = make([]model.Build, 0)
		buildmasterData = make([]bson.M, 0)
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData       projectContext
		VersionsByGitspec map[string]model.Version
		Builds            []model.Build
		VersionHistory    []bson.M
		User              *model.DBUser
	}{projCtx, gitspecMap, builds, buildmasterData, GetUser(r)}, "base", "buildmaster.html", "base_angular.html", "menu.html")
}
