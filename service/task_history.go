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
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
	Revision string    `json:"revision"`
	Message  string    `json:"message"`
	PushTime time.Time `json:"push_time"`
	// small amount of info about each task in this group
	TaskBlurb taskBlurb `json:"task"`
}

type versionDrawerItem struct {
	Revision string    `json:"revision"`
	Message  string    `json:"message"`
	PushTime time.Time `json:"push_time"`
	Id       string    `json:"version_id"`
	Errors   []string  `json:"errors"`
	Warnings []string  `json:"warnings"`
	Ignored  bool      `json:"ignored"`
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
	taskName := mux.Vars(r)["task_name"]

	var chunk model.TaskHistoryChunk
	var v *version.Version
	var before bool

	if strBefore := r.FormValue("before"); strBefore != "" {
		if before, err = strconv.ParseBool(strBefore); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	buildVariants := project.GetVariantsWithTask(taskName)

	if revision := r.FormValue("revision"); revision != "" {
		v, err = version.FindOne(version.ByProjectIdAndRevision(project.Identifier, revision))
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
		uis.WriteJSON(w, http.StatusOK, data)
		return
	default:
		uis.WriteHTML(w, http.StatusOK, struct {
			Data taskHistoryPageData
			ViewData
		}{data, uis.GetCommonViewData(w, r, false, true)}, "base",
			"task_history.html", "base_angular.html", "menu.html")
	}
}

func (uis *UIServer) variantHistory(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	variant := mux.Vars(r)["variant"]
	beforeCommitId := r.FormValue("before")
	isJson := (r.FormValue("format") == "json")

	var beforeCommit *version.Version
	var err error
	beforeCommit = nil
	if beforeCommitId != "" {
		beforeCommit, err = version.FindOne(version.ById(beforeCommitId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		grip.WarningWhen(beforeCommit == nil, "'before' was specified but query returned nil")
	}

	project, err := model.FindProject("", projCtx.ProjectRef)
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
		Versions  []version.Version
		Project   string
	}{variant, tasks, suites, versions, project.Identifier}
	if isJson {
		uis.WriteJSON(w, http.StatusOK, data)
		return
	}
	uis.WriteHTML(w, http.StatusOK, struct {
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

	taskName := mux.Vars(r)["task_name"]

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
	buildVariants := project.GetVariantsWithTask(taskName)

	onlyMatchingTasks := (r.FormValue("only_matching_tasks") == "true")

	// If there are no build variants, use all of them for the given task name.
	// Need this because without the build_variant specified, no amount of hinting
	// will get sort to use the proper index
	query := bson.M{
		"build_variant": bson.M{
			"$in": buildVariants,
		},
		"display_name": taskName,
		"order": bson.M{
			"$gte": lowOrder,
			"$lte": highOrder,
		},
		"branch": project.Identifier,
	}

	// If there are build variants, use them instead
	if len(filter.BuildVariants) > 0 {
		query["build_variant"] = bson.M{
			"$in": filter.BuildVariants,
		}
	}

	projection := bson.M{
		"_id":           1,
		"status":        1,
		"activated":     1,
		"time_taken":    1,
		"build_variant": 1,
	}

	last, err := task.Find(db.Query(query).Project(projection))

	if err != nil {
		http.Error(w, fmt.Sprintf("Error querying tasks: `%s`", err.Error()), http.StatusInternalServerError)
		return
	}

	for i := range last {
		if err := last[i].MergeNewTestResults(); err != nil {
			http.Error(w, fmt.Sprintf("Error merging test results: %s", err.Error()), http.StatusInternalServerError)
			return
		}
	}

	// do filtering of test results after we found the tasks that are requested
	for i := len(last) - 1; i >= 0; i-- {
		t := last[i]
		foundTest := false
		for j := len(t.LocalTestResults) - 1; j >= 0; j-- {
			result := t.LocalTestResults[j]
			// go through the test results and remove any that don't match the ones we care about
			status, exists := filter.Tests[result.TestFile]
			if !exists {
				// this test is not one we care about
				t.LocalTestResults = append(t.LocalTestResults[:j], t.LocalTestResults[j+1:]...)
				continue
			}
			if status != "ran" && status != result.Status {
				// this test is not in a status we care about
				t.LocalTestResults = append(t.LocalTestResults[:j], t.LocalTestResults[j+1:]...)
				continue
			}
			// we found the test in the correct status, so keep it
			foundTest = true
		}
		if !foundTest && onlyMatchingTasks {
			// if only want matching tasks and didn't find a match, remove the task
			last = append(last[:i], last[i+1:]...)
		}
	}

	uis.WriteJSON(w, http.StatusOK, last)
}

func (uis *UIServer) taskHistoryTestNames(w http.ResponseWriter, r *http.Request) {
	taskName := mux.Vars(r)["task_name"]

	projCtx := MustHaveProjectContext(r)

	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	buildVariants := project.GetVariantsWithTask(taskName)

	taskHistoryIterator := model.NewTaskHistoryIterator(taskName, buildVariants,
		project.Identifier)

	results, err := taskHistoryIterator.GetDistinctTestNames(NumTestsToSearchForTestNames)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error finding test names: `%v`", err.Error()), http.StatusInternalServerError)
		return
	}

	uis.WriteJSON(w, http.StatusOK, results)
}

// drawerParams contains the parameters from a request to populate a task or version history drawer.
type drawerParams struct {
	anchorId string
	window   string
	radius   int
}

func validateDrawerParams(r *http.Request) (drawerParams, error) {
	requestVars := mux.Vars(r)
	anchorId := requestVars["anchor"] // id of the item serving as reference point in history
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
	return drawerParams{anchorId, window, historyRadius}, nil
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
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Identifier,
		projCtx.Version.RevisionOrderNumber, drawerInfo.radius, projCtx.Version)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	versionDrawerItems := []versionDrawerItem{}
	for _, v := range versions {
		versionDrawerItems = append(versionDrawerItems, versionDrawerItem{
			v.Revision, v.Message, v.CreateTime, v.Id, v.Errors, v.Warnings, v.Ignored})
	}

	uis.WriteJSON(w, http.StatusOK, struct {
		Revisions []versionDrawerItem `json:"revisions"`
	}{versionDrawerItems})
}

// Handler for serving the data used to populate the task history drawer.
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
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Identifier,
		projCtx.Version.RevisionOrderNumber, drawerInfo.radius, projCtx.Version)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// populate task groups for the versions in the window
	taskGroups, err := getTaskDrawerItems(projCtx.Task.DisplayName, projCtx.Task.BuildVariant, false, versions)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteJSON(w, http.StatusOK, struct {
		Revisions []taskDrawerItem `json:"revisions"`
	}{taskGroups})
}

func getVersionsInWindow(wt, projectId string, anchorOrderNum, radius int,
	center *version.Version) ([]version.Version, error) {
	if wt == beforeWindow {
		return makeVersionsQuery(anchorOrderNum, projectId, radius, true)
	} else if wt == afterWindow {
		after, err := makeVersionsQuery(anchorOrderNum, projectId, radius, false)
		if err != nil {
			return nil, err
		}
		// reverse the versions in "after" so that they're ordered backwards in time
		for i, j := 0, len(after)-1; i < j; i, j = i+1, j-1 {
			after[i], after[j] = after[j], after[i]
		}
		return after, nil
	}
	before, err := makeVersionsQuery(anchorOrderNum, projectId, radius, true)
	if err != nil {
		return nil, err
	}
	after, err := makeVersionsQuery(anchorOrderNum, projectId, radius, false)
	if err != nil {
		return nil, err
	}
	// reverse the versions in "after" so that they're ordered backwards in time
	for i, j := 0, len(after)-1; i < j; i, j = i+1, j-1 {
		after[i], after[j] = after[j], after[i]
	}
	after = append(after, *center)
	after = append(after, before...)
	return after, nil
}

// Helper to make the appropriate query to the versions collection for what
// we will need.  "before" indicates whether to fetch versions before or
// after the passed-in task.
func makeVersionsQuery(anchorOrderNum int, projectId string, versionsToFetch int, before bool) ([]version.Version, error) {
	// decide how the versions we want relative to the task's revision order number
	ronQuery := bson.M{"$gt": anchorOrderNum}
	if before {
		ronQuery = bson.M{"$lt": anchorOrderNum}
	}

	// switch how to sort the versions
	sortVersions := []string{version.RevisionOrderNumberKey}
	if before {
		sortVersions = []string{"-" + version.RevisionOrderNumberKey}
	}

	// fetch the versions
	return version.Find(
		db.Query(bson.M{
			version.IdentifierKey:          projectId,
			version.RevisionOrderNumberKey: ronQuery,
			version.RequesterKey:           evergreen.RepotrackerVersionRequester,
		}).WithFields(
			version.RevisionOrderNumberKey,
			version.RevisionKey,
			version.MessageKey,
			version.CreateTimeKey,
			version.ErrorsKey,
			version.WarningsKey,
			version.IgnoredKey,
		).Sort(sortVersions).Limit(versionsToFetch))
}

// Given a task name and a slice of versions, return the appropriate sibling
// groups of tasks.  They will be sorted by ascending revision order number,
// unless reverseOrder is true, in which case they will be sorted
// descending.
func getTaskDrawerItems(displayName string, variant string, reverseOrder bool, versions []version.Version) ([]taskDrawerItem, error) {

	orderNumbers := make([]int, 0, len(versions))
	for _, v := range versions {
		orderNumbers = append(orderNumbers, v.RevisionOrderNumber)
	}

	revisionSort := task.RevisionOrderNumberKey
	if reverseOrder {
		revisionSort = "-" + revisionSort
	}

	tasks, err := task.Find(task.ByOrderNumbersForNameAndVariant(orderNumbers, displayName, variant).Sort([]string{revisionSort}))

	if err != nil {
		return nil, errors.Wrap(err, "error getting sibling tasks")
	}

	tasks, err = task.MergeTestResultsBulk(tasks)
	if err != nil {
		return nil, errors.Wrap(err, "error merging test results")
	}

	return createSiblingTaskGroups(tasks, versions), nil
}

// Given versions and the appropriate tasks within them, sorted by build
// variant, create sibling groups for the tasks.
func createSiblingTaskGroups(tasks []task.Task, versions []version.Version) []taskDrawerItem {
	// version id -> group
	groupsByVersion := map[string]taskDrawerItem{}

	// create a group for each version
	for _, v := range versions {
		group := taskDrawerItem{
			Revision: v.Revision,
			Message:  v.Message,
			PushTime: v.CreateTime,
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
