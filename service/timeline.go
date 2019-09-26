package service

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func (uis *UIServer) timelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, fmt.Sprintf("Error getting timeline data: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	skip, perPage := getSkipAndLimit(r, DefaultSkip, DefaultLimit)
	data, err := getTimelineData(project.Identifier, skip, perPage)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting timeline data: %v", err.Error()), http.StatusInternalServerError)
		return
	}

	gimlet.WriteJSON(w, data)
}

func (uis *UIServer) timeline(w http.ResponseWriter, r *http.Request) {
	uis.render.WriteResponse(w, http.StatusOK, uis.GetCommonViewData(w, r, false, true), "base", "timeline.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) patchTimeline(w http.ResponseWriter, r *http.Request) {
	uis.patchTimelineWrapper("", w, r)
}

func (uis *UIServer) myPatchesTimeline(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	if user.Settings.UseSpruceOptions.PatchPage {
		http.Redirect(w, r, fmt.Sprintf("%s/patches?user=%s", uis.Settings.Ui.UIv2Url, user.Username()), http.StatusTemporaryRedirect)
		return
	}

	uis.patchTimelineWrapper(user.Username(), w, r)
}

func (uis *UIServer) userPatchesTimeline(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	author := gimlet.GetVars(r)["user_id"]
	if user.Settings.UseSpruceOptions.PatchPage {
		http.Redirect(w, r, fmt.Sprintf("%s/patches?user=%s", uis.Settings.Ui.UIv2Url, author), http.StatusTemporaryRedirect)
		return
	}

	uis.patchTimelineWrapper(author, w, r)
}

func (uis *UIServer) patchTimelineWrapper(author string, w http.ResponseWriter, r *http.Request) {
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Author string
		ViewData
	}{author, uis.GetCommonViewData(w, r, false, true)}, "base", "patches.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) patchTimelineJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err,
			"Error fetching project for %v", project.Identifier))
		return
	}

	pageNum, err := strconv.Atoi(r.FormValue("page"))
	if err != nil {
		pageNum = 0
	}
	skip := pageNum * DefaultLimit

	user := gimlet.GetVars(r)["user_id"]
	var patches []patch.Patch
	if len(user) > 0 {
		patches, err = patch.Find(patch.ByUser(user).
			Project(patch.ExcludePatchDiff).
			Sort([]string{"-" + patch.CreateTimeKey}).
			Skip(skip).Limit(DefaultLimit))
	} else {
		patches, err = patch.Find(patch.ByProject(project.Identifier).
			Sort([]string{"-" + patch.CreateTimeKey}).
			Project(patch.ExcludePatchDiff).
			Skip(skip).Limit(DefaultLimit))
	}
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err,
			"Error fetching patches for %v", project.Identifier))
		return
	}

	versionIds := make([]string, 0, len(patches))
	uiPatches := make([]uiPatch, 0, len(patches))
	for _, patch := range patches {
		if patch.Version != "" {
			versionIds = append(versionIds, patch.Version)
		}
		var baseVersion *model.Version
		baseVersion, err = model.VersionFindOne(model.VersionByProjectIdAndRevision(patch.Project, patch.Githash))
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
	versions, err := model.VersionFind(model.VersionByIds(versionIds).WithoutFields(model.VersionConfigKey))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching versions for patches"))
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

	gimlet.WriteJSON(w, data)
}
