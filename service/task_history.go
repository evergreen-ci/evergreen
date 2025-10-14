package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	beforeWindow = "before"
	afterWindow  = "after"

	// Initial number of revisions to return on first page load
	InitRevisionsBefore = 50
	InitRevisionsAfter  = 100

	// Number of revisions to return on subsequent requests
	NoRevisions     = 0
	MaxNumRevisions = 50
)

// Representation of a group of tasks with the same display name and revision,
// but different build variants.
type taskDrawerItem struct {
	Revision   string    `json:"revision"`
	VersionID  string    `json:"version_id"`
	Message    string    `json:"message"`
	CreateTime time.Time `json:"create_time"`
	// small amount of info about each task in this group
	TaskBlurb taskBlurb `json:"task"`
}

type versionDrawerItem struct {
	Revision   string    `json:"revision"`
	Message    string    `json:"message"`
	CreateTime time.Time `json:"create_time"`
	Id         string    `json:"version_id"`
	Errors     []string  `json:"errors"`
	Warnings   []string  `json:"warnings"`
	Ignored    bool      `json:"ignored"`
}

// Represents a small amount of information about a task - used as part of the
// task history to display a visual blurb.
type taskBlurb struct {
	Id       string                  `json:"id"`
	Variant  string                  `json:"variant"`
	Status   string                  `json:"status"`
	Details  apimodels.TaskEndDetail `json:"task_end_details"`
	Failures []string                `json:"failures"`
}


func (uis *UIServer) variantHistory(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	variant := gimlet.GetVars(r)["variant"]
	beforeCommitId := r.FormValue("before")
	isJson := (r.FormValue("format") == "json")

	var beforeCommit *model.Version
	var err error
	beforeCommit = nil
	if beforeCommitId != "" {
		beforeCommit, err = model.VersionFindOne(r.Context(), model.VersionById(beforeCommitId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		grip.WarningWhen(beforeCommit == nil, "'before' was specified but query returned nil")
	}

	_, project, _, err := model.FindLatestVersionWithValidProject(projCtx.ProjectRef.Id, false)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	bv := project.FindBuildVariant(variant)
	if bv == nil {
		http.Error(w, "variant not found", http.StatusNotFound)
		return
	}

	iter := model.NewBuildVariantHistoryIterator(variant, bv.Name, project.Identifier)
	tasks, versions, err := iter.GetItems(r.Context(), beforeCommit, 50)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	var suites []string
	for _, task := range bv.Tasks {
		suites = append(suites, task.Name)
	}

	sort.Strings(suites)

	data := struct {
		Variant   string
		Tasks     []bson.M
		TaskNames []string
		Versions  []model.Version
		Project   string
	}{variant, tasks, suites, versions, project.Identifier}
	if isJson {
		gimlet.WriteJSON(w, data)
		return
	}
	uis.render.WriteResponse(w, http.StatusOK, struct {
		Data any
		ViewData
	}{data, uis.GetCommonViewData(w, r, false, true)}, "base",
		"build_variant_history.html", "base_angular.html", "menu.html")
}

// drawerParams contains the parameters from a request to populate a task or version history drawer.
type drawerParams struct {
	anchorId    string // id of the item serving as reference point in history
	window      string
	radius      int
	displayName string
	variant     string
}

func validateDrawerParams(r *http.Request) (drawerParams, error) {
	requestVars := gimlet.GetVars(r)
	window := requestVars["window"]

	// do some validation on the window of tasks requested
	if window != "surround" && window != "before" && window != "after" {
		return drawerParams{}, errors.Errorf("invalid value %v for window", window)
	}

	// the 'radius' of the history we want (how many tasks on each side of the anchor task)
	radius := r.FormValue("radius")
	if radius == "" {
		radius = "5"
	}
	historyRadius, err := strconv.Atoi(radius)
	if err != nil {
		return drawerParams{}, errors.Errorf("invalid value %v for radius", radius)
	}
	return drawerParams{
		anchorId:    requestVars["anchor"],
		window:      window,
		radius:      historyRadius,
		displayName: requestVars["display_name"],
		variant:     requestVars["variant"],
	}, nil
}

// Handler for serving the data used to populate the task history drawer.
func (uis *UIServer) versionHistoryDrawer(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	drawerInfo, err := validateDrawerParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// get the versions in the requested window
	versions, err := getVersionsInWindow(r.Context(), drawerInfo.window, projCtx.Version.Identifier, drawerInfo.radius, projCtx.Version)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	versionDrawerItems := []versionDrawerItem{}
	for _, v := range versions {
		versionDrawerItems = append(versionDrawerItems, versionDrawerItem{
			Revision:   v.Revision,
			Message:    v.Message,
			CreateTime: v.CreateTime,
			Id:         v.Id,
			Errors:     v.Errors,
			Warnings:   v.Warnings,
			Ignored:    v.Ignored,
		})
	}

	gimlet.WriteJSON(w, struct {
		Revisions []versionDrawerItem `json:"revisions"`
	}{versionDrawerItems})
}

// Handler for serving the data used to populate the task history drawer
func (uis *UIServer) taskHistoryDrawer(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	drawerInfo, err := validateDrawerParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if projCtx.Version == nil {
		http.Error(w, "no version available", http.StatusBadRequest)
		return
	}
	// get the versions in the requested window
	versions, err := getVersionsInWindow(r.Context(), drawerInfo.window, projCtx.Version.Identifier, drawerInfo.radius, projCtx.Version)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// populate task groups for the versions in the window
	taskGroups, err := getTaskDrawerItems(r.Context(), drawerInfo.displayName, drawerInfo.variant, false, versions)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, struct {
		Revisions []taskDrawerItem `json:"revisions"`
	}{taskGroups})
}

// Get the versions for projectID within radius around the center version, sorted backwards in time
// wt indicates the direction away from center
func getVersionsInWindow(ctx context.Context, wt, projectID string, radius int, center *model.Version) ([]model.Version, error) {
	if wt == beforeWindow {
		return surroundingVersions(ctx, center, projectID, radius, true)
	} else if wt == afterWindow {
		after, err := surroundingVersions(ctx, center, projectID, radius, false)
		if err != nil {
			return nil, err
		}
		return after, nil
	}
	before, err := surroundingVersions(ctx, center, projectID, radius, true)
	if err != nil {
		return nil, err
	}
	after, err := surroundingVersions(ctx, center, projectID, radius, false)
	if err != nil {
		return nil, err
	}

	return append(append(after, *center), before...), nil
}

// Helper to query the versions collection for versions created before
// or after the center, indicated by "before", and sorted backwards in time
// Results are sorted on create_time and revision order number, similar to the waterfall
func surroundingVersions(ctx context.Context, center *model.Version, projectId string, versionsToFetch int, before bool) ([]model.Version, error) {
	direction := "$gt"
	sortOnConsecutive := []string{model.VersionCreateTimeKey, model.VersionRevisionOrderNumberKey}
	sortOnConcurrent := []string{model.VersionRevisionOrderNumberKey}
	if before {
		direction = "$lt"
		sortOnConsecutive = []string{"-" + model.VersionCreateTimeKey, "-" + model.VersionRevisionOrderNumberKey}
		sortOnConcurrent = []string{"-" + model.VersionRevisionOrderNumberKey}
	}

	// Break the query in two: concurrent and consecutive versions.
	// Since we know they can just be concatenated this allows us to use the index efficiently

	// fetch concurrent versions
	versions, err := model.VersionFind(ctx,
		db.Query(bson.M{
			model.VersionIdentifierKey: projectId,
			model.VersionCreateTimeKey: center.CreateTime,
			model.VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			model.VersionRevisionOrderNumberKey: bson.M{direction: center.RevisionOrderNumber},
		}).WithFields(
			model.VersionRevisionOrderNumberKey,
			model.VersionRevisionKey,
			model.VersionMessageKey,
			model.VersionCreateTimeKey,
			model.VersionErrorsKey,
			model.VersionWarningsKey,
			model.VersionIgnoredKey,
		).Sort(sortOnConcurrent).Limit(versionsToFetch))
	if err != nil {
		return nil, errors.Wrap(err, "can't get concurrent versions")
	}

	if len(versions) < versionsToFetch {
		// fetch consecutive versions
		consecutiveVersions, err := model.VersionFind(ctx,
			db.Query(bson.M{
				model.VersionIdentifierKey: projectId,
				model.VersionCreateTimeKey: bson.M{direction: center.CreateTime},
				model.VersionRequesterKey: bson.M{
					"$in": evergreen.SystemVersionRequesterTypes,
				},
			}).WithFields(
				model.VersionRevisionOrderNumberKey,
				model.VersionRevisionKey,
				model.VersionMessageKey,
				model.VersionCreateTimeKey,
				model.VersionErrorsKey,
				model.VersionWarningsKey,
				model.VersionIgnoredKey,
			).Sort(sortOnConsecutive).Limit(versionsToFetch-len(versions)))
		if err != nil {
			return nil, errors.Wrap(err, "can't get consecutive versions")
		}

		versions = append(versions, consecutiveVersions...)
	}

	if before {
		return versions, nil
	}

	// reverse versions
	for begin, end := 0, len(versions)-1; begin < end; begin, end = begin+1, end-1 {
		versions[begin], versions[end] = versions[end], versions[begin]
	}
	return versions, nil
}

// Given a task name and a slice of versions, return the appropriate sibling
// groups of tasks.  They will be sorted by ascending revision order number,
// unless reverseOrder is true, in which case they will be sorted
// descending.
func getTaskDrawerItems(ctx context.Context, displayName string, variant string, reverseOrder bool, versions []model.Version) ([]taskDrawerItem, error) {
	versionIds := []string{}
	for _, v := range versions {
		versionIds = append(versionIds, v.Id)
	}

	revisionSort := task.RevisionOrderNumberKey
	if reverseOrder {
		revisionSort = "-" + revisionSort
	}

	q := db.Query(task.ByVersionsForNameAndVariant(versionIds, []string{displayName}, variant)).Sort([]string{revisionSort})
	tasks, err := task.FindAll(ctx, q)

	if err != nil {
		return nil, errors.Wrap(err, "error getting sibling tasks")
	}

	taskIds := []string{}
	for _, t := range tasks {
		if evergreen.IsFailedTaskStatus(t.Status) {
			taskIds = append(taskIds, t.Id)
			taskIds = append(taskIds, t.ExecutionTasks...) // also add test results of exuection tasks to the parent
		}
	}

	return createSiblingTaskGroups(tasks, versions), nil
}

// Given versions and the appropriate tasks within them, sorted by build
// variant, create sibling groups for the tasks.
func createSiblingTaskGroups(tasks []task.Task, versions []model.Version) []taskDrawerItem {
	// version id -> group
	groupsByVersion := map[string]taskDrawerItem{}

	// create a group for each version
	for _, v := range versions {
		group := taskDrawerItem{
			Revision:   v.Revision,
			Message:    v.Message,
			CreateTime: v.CreateTime,
			VersionID:  v.Id,
		}
		groupsByVersion[v.Id] = group
	}

	// go through the tasks, adding blurbs for them to the appropriate versions
	for _, task := range tasks {
		blurb := taskBlurb{
			Id:      task.Id,
			Variant: task.BuildVariant,
			Status:  task.Status,
			Details: task.Details,
		}

		for _, result := range task.LocalTestResults {
			if result.Status == evergreen.TestFailedStatus {
				blurb.Failures = append(blurb.Failures, result.TestName)
			}
		}

		// add the blurb to the appropriate group
		groupForVersion := groupsByVersion[task.Version]
		groupForVersion.TaskBlurb = blurb
		groupsByVersion[task.Version] = groupForVersion
	}

	// create a slice of the sibling groups, in the appropriate order
	orderedGroups := make([]taskDrawerItem, 0, len(versions))
	for _, version := range versions {
		orderedGroups = append(orderedGroups, groupsByVersion[version.Id])
	}

	return orderedGroups

}
