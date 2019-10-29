package service

import (
	"encoding/json"
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
	"github.com/evergreen-ci/evergreen/model/testresult"
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

// Serves the task history page itself.
func (uis *UIServer) taskHistoryPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	taskName := gimlet.GetVars(r)["task_name"]

	var chunk model.TaskHistoryChunk
	var v *model.Version
	var before bool

	if strBefore := r.FormValue("before"); strBefore != "" {
		if before, err = strconv.ParseBool(strBefore); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	repo, err := model.FindRepository(project.Identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	buildVariants, err := task.FindVariantsWithTask(taskName, project.Identifier, repo.RevisionOrderNumber-50, repo.RevisionOrderNumber)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if revision := r.FormValue("revision"); revision != "" {
		v, err = model.VersionFindOne(model.VersionByProjectIdAndRevision(project.Identifier, revision))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	taskHistoryIterator := model.NewTaskHistoryIterator(taskName, buildVariants, project.Identifier)

	if r.FormValue("format") == "" {
		if v != nil {
			chunk, err = taskHistoryIterator.GetChunk(v, InitRevisionsBefore, InitRevisionsAfter, true)
		} else {
			// Load the most recent MaxNumRevisions if a particular
			// version was unspecified
			chunk, err = taskHistoryIterator.GetChunk(v, MaxNumRevisions, NoRevisions, false)
		}
	} else if before {
		chunk, err = taskHistoryIterator.GetChunk(v, MaxNumRevisions, NoRevisions, false)
	} else {
		chunk, err = taskHistoryIterator.GetChunk(v, NoRevisions, MaxNumRevisions, false)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := taskHistoryPageData{
		TaskName:         taskName,
		Tasks:            chunk.Tasks,
		Variants:         buildVariants,
		FailedTests:      chunk.FailedTests,
		Versions:         chunk.Versions,
		ExhaustedBefore:  chunk.Exhausted.Before,
		ExhaustedAfter:   chunk.Exhausted.After,
		SelectedRevision: r.FormValue("revision"),
	}

	switch r.FormValue("format") {
	case "json":
		gimlet.WriteJSON(w, data)
		return
	default:
		uis.render.WriteResponse(w, http.StatusOK, struct {
			Data taskHistoryPageData
			ViewData
		}{data, uis.GetCommonViewData(w, r, false, true)}, "base",
			"task_history.html", "base_angular.html", "menu.html")
	}
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
		beforeCommit, err = model.VersionFindOne(model.VersionById(beforeCommitId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		grip.WarningWhen(beforeCommit == nil, "'before' was specified but query returned nil")
	}

	project, err := model.FindLastKnownGoodProject(projCtx.ProjectRef.Identifier)
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
	tasks, versions, err := iter.GetItems(beforeCommit, 50)
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
		Data interface{}
		ViewData
	}{data, uis.GetCommonViewData(w, r, false, true)}, "base",
		"build_variant_history.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) taskHistoryPickaxe(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	taskName := gimlet.GetVars(r)["task_name"]

	highOrder, err := strconv.ParseInt(r.FormValue("high"), 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing high: `%s`", err.Error()), http.StatusBadRequest)
		return
	}
	lowOrder, err := strconv.ParseInt(r.FormValue("low"), 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing low: `%s`", err.Error()), http.StatusBadRequest)
		return
	}

	filter := struct {
		BuildVariants []string          `json:"buildVariants"`
		Tests         map[string]string `json:"tests"`
	}{}

	err = json.Unmarshal([]byte(r.FormValue("filter")), &filter)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error in filter: %v", err.Error()), http.StatusBadRequest)
		return
	}

	onlyMatchingTasks := (r.FormValue("only_matching_tasks") == "true")

	params := model.PickaxeParams{
		Project:           project,
		TaskName:          taskName,
		NewestOrder:       highOrder,
		OldestOrder:       lowOrder,
		BuildVariants:     filter.BuildVariants,
		Tests:             filter.Tests,
		OnlyMatchingTasks: onlyMatchingTasks,
	}
	tasks, err := model.TaskHistoryPickaxe(params)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding tasks: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	gimlet.WriteJSON(w, tasks)
}

func (uis *UIServer) taskHistoryTestNames(w http.ResponseWriter, r *http.Request) {
	taskName := gimlet.GetVars(r)["task_name"]

	projCtx := MustHaveProjectContext(r)

	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	repo, err := model.FindRepository(project.Identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	buildVariants, err := task.FindVariantsWithTask(taskName, project.Identifier, repo.RevisionOrderNumber-50, repo.RevisionOrderNumber)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	taskHistoryIterator := model.NewTaskHistoryIterator(taskName, buildVariants,
		project.Identifier)

	results, err := taskHistoryIterator.GetDistinctTestNames(NumTestsToSearchForTestNames)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding test names: `%v`", err.Error()), http.StatusInternalServerError)
		return
	}

	gimlet.WriteJSON(w, results)
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
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Identifier, drawerInfo.radius, projCtx.Version)
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
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Identifier, drawerInfo.radius, projCtx.Version)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// populate task groups for the versions in the window
	taskGroups, err := getTaskDrawerItems(drawerInfo.displayName, drawerInfo.variant, false, versions)
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
func getVersionsInWindow(wt, projectID string, radius int, center *model.Version) ([]model.Version, error) {
	if wt == beforeWindow {
		return surroundingVersions(center, projectID, radius, true)
	} else if wt == afterWindow {
		after, err := surroundingVersions(center, projectID, radius, false)
		if err != nil {
			return nil, err
		}
		return after, nil
	}
	before, err := surroundingVersions(center, projectID, radius, true)
	if err != nil {
		return nil, err
	}
	after, err := surroundingVersions(center, projectID, radius, false)
	if err != nil {
		return nil, err
	}

	return append(append(after, *center), before...), nil
}

// Helper to query the versions collection for versions created before
// or after the center, indicated by "before", and sorted backwards in time
func surroundingVersions(center *model.Version, projectID string, versionsToFetch int, before bool) ([]model.Version, error) {
	direction := "$gt"
	sortOn := []string{model.VersionCreateTimeKey, model.VersionRevisionOrderNumberKey}
	if before {
		direction = "$lt"
		sortOn = []string{"-" + model.VersionCreateTimeKey, "-" + model.VersionRevisionOrderNumberKey}
	}

	versions, err := model.VersionFind(
		db.Query(bson.M{
			model.VersionIdentifierKey: projectID,
			model.VersionRequesterKey: bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
			"$or": []bson.M{
				{model.VersionCreateTimeKey: bson.M{direction: center.CreateTime}},
				{model.VersionCreateTimeKey: center.CreateTime, model.VersionRevisionOrderNumberKey: bson.M{direction: center.RevisionOrderNumber}},
			},
		}).WithFields(
			model.VersionRevisionOrderNumberKey,
			model.VersionRevisionKey,
			model.VersionMessageKey,
			model.VersionCreateTimeKey,
			model.VersionErrorsKey,
			model.VersionWarningsKey,
			model.VersionIgnoredKey,
		).Sort(sortOn).Limit(versionsToFetch))
	if err != nil {
		return nil, errors.Wrap(err, "can't get surrounding versions")
	}

	if !before {
		// reverse versions
		for begin, end := 0, len(versions)-1; begin < end; begin, end = begin+1, end-1 {
			versions[begin], versions[end] = versions[end], versions[begin]
		}
	}

	return versions, nil
}

// Given a task name and a slice of versions, return the appropriate sibling
// groups of tasks.  They will be sorted by ascending revision order number,
// unless reverseOrder is true, in which case they will be sorted
// descending.
func getTaskDrawerItems(displayName string, variant string, reverseOrder bool, versions []model.Version) ([]taskDrawerItem, error) {

	versionIds := []string{}
	for _, v := range versions {
		versionIds = append(versionIds, v.Id)
	}

	revisionSort := task.RevisionOrderNumberKey
	if reverseOrder {
		revisionSort = "-" + revisionSort
	}

	tasks, err := task.FindWithDisplayTasks(task.ByVersionsForNameAndVariant(versionIds, []string{displayName}, variant).Sort([]string{revisionSort}))

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
	query := db.Query(bson.M{
		testresult.TaskIDKey: bson.M{
			"$in": taskIds,
		},
		testresult.StatusKey: evergreen.TestFailedStatus,
	})
	tasks, err = task.MergeTestResultsBulk(tasks, &query)
	if err != nil {
		return nil, errors.Wrap(err, "error merging test results")
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
				blurb.Failures = append(blurb.Failures, result.TestFile)
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
