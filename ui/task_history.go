package ui

import (
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"sort"
	"strconv"
	"time"
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

	// this regex either matches against the exact 'test' string, or
	// against the 'test' string at the end of some kind of filepath.
	testMatchRegex = `(\Q%s\E|.*(\\|/)\Q%s\E)$`
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

	if projCtx.Project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	taskName := mux.Vars(r)["task_name"]

	var chunk model.TaskHistoryChunk
	var v *version.Version
	var before bool
	var err error

	if strBefore := r.FormValue("before"); strBefore != "" {
		if before, err = strconv.ParseBool(strBefore); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	buildVariants := projCtx.Project.GetVariantsWithTask(taskName)

	if revision := r.FormValue("revision"); revision != "" {
		v, err = version.FindOne(version.ByProjectIdAndRevision(projCtx.Project.Identifier, revision))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	taskHistoryIterator := model.NewTaskHistoryIterator(taskName, buildVariants, projCtx.Project.Identifier)

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
			ProjectData projectContext
			User        *user.DBUser
			Flashes     []interface{}
			Data        taskHistoryPageData
		}{projCtx, GetUser(r), []interface{}{}, data}, "base",
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
		if beforeCommit == nil {
			evergreen.Logger.Logf(slogger.WARN, "'before' was specified but query returned nil")
		}
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

	iter := model.NewBuildVariantHistoryIterator(variant, bv.Name, projCtx.Project.Identifier)
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
	}{variant, tasks, suites, versions, projCtx.Project.Identifier}
	if isJson {
		uis.WriteJSON(w, http.StatusOK, data)
		return
	}
	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Flashes     []interface{}
		Data        interface{}
	}{projCtx, GetUser(r), []interface{}{}, data}, "base",
		"build_variant_history.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) taskHistoryPickaxe(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Project == nil {
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
	buildVariants := projCtx.Project.GetVariantsWithTask(taskName)

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
		"branch": projCtx.Project.Identifier,
	}

	// If there are build variants, use them instead
	if len(filter.BuildVariants) > 0 {
		query["build_variant"] = bson.M{
			"$in": filter.BuildVariants,
		}
	}

	// If there are tests to filter by, create a big $elemMatch $or in the
	// projection to make sure we only get the tests we care about.
	elemMatchOr := make([]bson.M, 0)
	for test, result := range filter.Tests {
		regexp := fmt.Sprintf(testMatchRegex, test, test)
		if result == "ran" {
			// Special case: if asking for tasks where the test ran, don't care
			// about the test status
			elemMatchOr = append(elemMatchOr, bson.M{
				"test_file": bson.RegEx{regexp, ""},
			})
		} else {
			elemMatchOr = append(elemMatchOr, bson.M{
				"test_file": bson.RegEx{regexp, ""},
				"status":    result,
			})
		}
	}

	elemMatch := bson.M{"$or": elemMatchOr}

	// Special case: if only one test filter, don't need to use a $or
	if 1 == len(elemMatchOr) {
		elemMatch = elemMatchOr[0]
	}

	projection := bson.M{
		"_id":           1,
		"status":        1,
		"activated":     1,
		"time_taken":    1,
		"build_variant": 1,
	}

	if len(elemMatchOr) > 0 {
		projection["test_results"] = bson.M{
			"$elemMatch": elemMatch,
		}

		// If we only care about matching tasks, put the elemMatch in the query too
		if onlyMatchingTasks {
			query["test_results"] = bson.M{
				"$elemMatch": elemMatch,
			}
		}
	}

	last, err := task.Find(db.Query(query).Project(projection))

	if err != nil {
		http.Error(w, fmt.Sprintf("Error querying tasks: `%s`", err.Error()), http.StatusInternalServerError)
		return
	}

	uis.WriteJSON(w, http.StatusOK, last)
}

func (uis *UIServer) taskHistoryTestNames(w http.ResponseWriter, r *http.Request) {
	taskName := mux.Vars(r)["task_name"]

	projCtx := MustHaveProjectContext(r)

	if projCtx.Project == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	buildVariants := projCtx.Project.GetVariantsWithTask(taskName)

	taskHistoryIterator := model.NewTaskHistoryIterator(taskName, buildVariants,
		projCtx.Project.Identifier)

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
		return drawerParams{}, fmt.Errorf("invalid value %v for window", window)
	}

	// the 'radius' of the history we want (how many tasks on each side of the anchor task)
	radius := r.FormValue("radius")
	if radius == "" {
		radius = "5"
	}
	historyRadius, err := strconv.Atoi(radius)
	if err != nil {
		return drawerParams{}, fmt.Errorf("invalid value %v for radius", radius)
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
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Id, projCtx.Version.Identifier,
		projCtx.Version.RevisionOrderNumber, drawerInfo.radius, projCtx.Version)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	versionDrawerItems := []versionDrawerItem{}
	for _, v := range versions {
		versionDrawerItems = append(versionDrawerItems, versionDrawerItem{v.Revision, v.Message, v.CreateTime, v.Id})
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

	// get the versions in the requested window
	versions, err := getVersionsInWindow(drawerInfo.window, projCtx.Version.Id, projCtx.Version.Identifier,
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

func getVersionsInWindow(wt, anchorId, projectId string, anchorOrderNum, radius int,
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
		}).WithFields(
			version.RevisionOrderNumberKey,
			version.RevisionKey,
			version.MessageKey,
			version.CreateTimeKey,
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
		return nil, fmt.Errorf("error getting sibling tasks: %v", err)
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

		for _, result := range task.TestResults {
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
