package service

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type PatchInfo struct {
	Id            string    `json:"id"`
	Version       string    `json:"version"`
	Author        string    `json:"author"`
	CreateTime    time.Time `json:"create_time"`
	Project       string    `json:"project"`
	Description   string    `json:"description"`
	Githash       string    `json:"githash"`
	BaseVersionId string    `json:"base_version_id"`
	Alias         string    `json:"alias"`
}

type BuildInfo struct {
	Id          string     `json:"id"`
	DisplayName string     `json:"display_name"`
	Tasks       []TaskInfo `json:"tasks"`
}
type TaskInfo struct {
	Id          string                  `json:"id"`
	DisplayName string                  `json:"display_name"`
	Status      string                  `json:"status"`
	Details     apimodels.TaskEndDetail `json:"status_details"`
}

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
	// if user.Settings.UseSpruceOptions.PatchPage {
	// 	http.Redirect(w, r, fmt.Sprintf("%s/patches?user=%s", uis.Settings.Ui.UIv2Url, user.Username()), http.StatusTemporaryRedirect)
	// 	return
	// }

	uis.patchTimelineWrapper(user.Username(), w, r)
}

func (uis *UIServer) userPatchesTimeline(w http.ResponseWriter, r *http.Request) {
	// user := MustHaveUser(r)
	author := gimlet.GetVars(r)["user_id"]
	// if user.Settings.UseSpruceOptions.PatchPage {
	// 	http.Redirect(w, r, fmt.Sprintf("%s/patches?user=%s", uis.Settings.Ui.UIv2Url, author), http.StatusTemporaryRedirect)
	// 	return
	// }

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
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err,
				"Error fetching patches for user %v", user))
			return
		}
	} else {
		projectID := gimlet.GetVars(r)["project_id"]
		project, err := projCtx.GetProject()
		if err != nil || project == nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err,
				"Error fetching project %v", projectID))
			return
		}
		patches, err = patch.Find(patch.ByProject(project.Identifier).
			Sort([]string{"-" + patch.CreateTimeKey}).
			Project(patch.ExcludePatchDiff).
			Skip(skip).Limit(DefaultLimit))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err,
				"Error fetching patches for project %v", project.Identifier))
			return
		}
	}

	versionIds := make([]string, 0, len(patches))
	uiPatches := make([]PatchInfo, 0, len(patches))
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

		uiPatches = append(uiPatches, PatchInfo{
			Id:            patch.Id.Hex(),
			Version:       patch.Version,
			Author:        patch.Author,
			CreateTime:    patch.CreateTime,
			Project:       patch.Project,
			Description:   patch.Description,
			Githash:       patch.Githash,
			BaseVersionId: baseVersionId,
			Alias:         patch.Alias,
		})
	}
	versions, err := model.VersionFind(model.VersionByIds(versionIds).WithoutFields(model.VersionConfigKey))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error fetching versions for patches"))
		return
	}

	buildsMap := map[string][]BuildInfo{}
	for _, version := range versions {
		builds, err := getBuildInfo(version.BuildIds)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		buildsMap[version.Id] = builds
	}

	data := struct {
		BuildsMap map[string][]BuildInfo
		UIPatches []PatchInfo
		PageNum   int
	}{buildsMap, uiPatches, pageNum}

	gimlet.WriteJSON(w, data)
}

func getBuildInfo(buildIds []string) ([]BuildInfo, error) {
	dbBuilds, err := build.Find(build.ByIds(buildIds))
	if err != nil {
		return nil, errors.Wrap(err, "can't get builds")
	}

	builds := make([]BuildInfo, 0, len(dbBuilds))
	for _, dbBuild := range dbBuilds {
		tasks := make([]TaskInfo, 0, len(dbBuild.Tasks))
		for _, task := range dbBuild.Tasks {
			tasks = append(tasks, TaskInfo{
				Id:          task.Id,
				DisplayName: task.DisplayName,
				Status:      task.Status,
				Details:     task.StatusDetails,
			})
		}
		builds = append(builds, BuildInfo{
			Id:          dbBuild.Id,
			DisplayName: dbBuild.DisplayName,
			Tasks:       tasks,
		})
	}

	return builds, nil
}
