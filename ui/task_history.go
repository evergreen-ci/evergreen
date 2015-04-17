package ui

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/user"
	"10gen.com/mci/model/version"
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Representation of a group of tasks with the same display name and revision,
// but different build variants.
type siblingTaskGroup struct {
	// revision-level info
	Revision string    `json:"revision"`
	Message  string    `json:"message"`
	PushTime time.Time `json:"push_time"`

	// small amount of info about each appropriate task
	TaskBlurbs []taskBlurb `json:"tasks"`
}

// Represents a small amount of information about a task - used as part of the
// task history to display a visual blurb.
type taskBlurb struct {
	Id       string   `json:"id"`
	Variant  string   `json:"variant"`
	Status   string   `json:"status"`
	Failures []string `json:"failures"`
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
			mci.Logger.Logf(slogger.WARN, "'before' was specified but query returned nil")
		}
	}

	project, err := model.FindProject("", projCtx.Project.Identifier, uis.MCISettings.ConfigDir)
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
		regexp := fmt.Sprintf(".*(\\\\|/)%s$", test)
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

	last, err := model.FindAllTasks(query, projection, []string{}, 0, 0)

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

// Handler for serving json representing a task history.  Is based on the
// task used as an anchor for the point in history, as well as whether the
// history is taken to mean tasks before, after, or surrounding the task in
// time.
func (uis *UIServer) taskHistoryJson(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	// get the request vars
	requestVars := mux.Vars(r)

	// get the id of the task serving as our reference point for the history
	anchorTaskId := requestVars["anchor"]

	// whether we are fetching tasks from versions before, after, or surrounding the id
	window := requestVars["window"]

	// do some validation on the window of tasks requested
	if window != "surround" && window != "before" && window != "after" {
		http.Error(w, fmt.Sprintf("invalid value %v for window", window), http.StatusBadRequest)
		return
	}

	// the 'radius' of the history we want (how many tasks on each side of the anchor task)
	radius := r.FormValue("radius")
	if radius == "" {
		radius = "5"
	}
	historyRadius, err := strconv.Atoi(radius)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid value %v for radius", radius), http.StatusBadRequest)
		return
	}

	variantMappings := projCtx.Project.GetVariantMappings()

	// fetch the appropriate task groups
	var tasksWithinWindow []siblingTaskGroup
	switch window {
	case "surround":
		tasksWithinWindow, err = getGroupsSurrounding(projCtx.Task, historyRadius)
	case "before":
		tasksWithinWindow, err = getGroupsBefore(projCtx.Task, historyRadius)
	case "after":
		tasksWithinWindow, err = getGroupsAfter(projCtx.Task, historyRadius)
		// can't hit another value since we validate 'window' above
	}

	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "error finding history in reference"+
			" to task %v: %v", anchorTaskId, err)
		http.Error(w, fmt.Sprintf("error finding history for task %v with window type of %v: %v",
			projCtx.Task.Id, window, err), http.StatusBadRequest)
		return
	}

	data := struct {
		Revisions       []siblingTaskGroup `json:"revisions"`
		VariantMappings map[string]string  `json:"variant_mappings"`
	}{tasksWithinWindow, variantMappings}

	// Representation of the necessary data to build the task history
	type taskHistoryInfo struct {
	}

	uis.WriteJSON(w, http.StatusOK, data)
}

// Get all of the sibling task groups for the revisions surrounding the one
// containing the passed-in task.
func getGroupsSurrounding(anchorTask *model.Task,
	historyRadius int) ([]siblingTaskGroup, error) {

	// first, get the sibling group for the version containing the
	// anchor task itself

	anchorVersion, err := version.FindOne(version.ById(anchorTask.Version))
	if err != nil {
		return nil, fmt.Errorf("error finding version %v: %v",
			anchorTask.Version, err)
	}

	if anchorVersion == nil {
		return nil, fmt.Errorf("no such version %v", anchorTask.Version)
	}

	// len(anchorSiblingGroupSlice) should always be 1, since it's only
	// getting a group for the single relevant revision.
	anchorSiblingGroupSlice, err := getSiblingTaskGroups(anchorTask.DisplayName,
		false, []version.Version{*anchorVersion})

	if err != nil {
		return nil, fmt.Errorf("error getting sibling tasks: %v", err)
	}

	// then, get the appropriate tasks before the anchor in time

	siblingGroupsBefore, err := getGroupsBefore(anchorTask, historyRadius)

	if err != nil {
		return nil, fmt.Errorf("error getting sibling groups before %v: %v",
			anchorTask.Id, err)
	}

	// then, get the appropriate tasks after the anchor in time

	siblingGroupsAfter, err := getGroupsAfter(anchorTask, historyRadius)

	if err != nil {
		return nil, fmt.Errorf("error getting sibling groups after %v: %v",
			anchorTask.Id, err)
	}

	// merge the groups

	fullGroupList := mergeGroups(siblingGroupsAfter, anchorSiblingGroupSlice,
		siblingGroupsBefore)

	return fullGroupList, nil
}

// Get the sibling groups for the revisions before the one containing the
// specified task.
func getGroupsBefore(anchorTask *model.Task,
	numGroupsWanted int) ([]siblingTaskGroup, error) {

	// first, get the relevant versions we will need
	versions, err := makeVersionsQuery(anchorTask, numGroupsWanted, true)
	if err != nil {
		return nil, fmt.Errorf("error getting versions before task: %v", err)
	}

	// now, get the appropriate sibling task groups
	return getSiblingTaskGroups(anchorTask.DisplayName, true, versions)
}

// Get the sibling groups for the revisions after the one containing the
// specified task.
func getGroupsAfter(anchorTask *model.Task,
	numGroupsWanted int) ([]siblingTaskGroup, error) {

	// first, get the relevant versions we will need
	versions, err := makeVersionsQuery(anchorTask, numGroupsWanted, false)
	if err != nil {
		return nil, fmt.Errorf("error getting versions after: %v", err)
	}

	// reverse the versions so that they're ordered backwards in time
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	// now, get the appropriate sibling task groups
	return getSiblingTaskGroups(anchorTask.DisplayName, true, versions)
}

// Helper to make the appropriate query to the versions collection for what
// we will need.  "before" indicates whether to fetch versions before or
// after the passed-in task.
func makeVersionsQuery(anchorTask *model.Task, versionsToFetch int, before bool) ([]version.Version, error) {
	// decide how the versions we want relative to the task's revision order
	// number
	ronQuery := bson.M{"$gt": anchorTask.RevisionOrderNumber}
	if before {
		ronQuery = bson.M{"$lt": anchorTask.RevisionOrderNumber}
	}

	// switch how to sort the versions
	sortVersions := []string{version.RevisionOrderNumberKey}
	if before {
		sortVersions = []string{"-" + version.RevisionOrderNumberKey}
	}

	// fetch the versions
	return version.Find(
		db.Query(bson.M{
			version.ProjectKey:             anchorTask.Project,
			version.RevisionOrderNumberKey: ronQuery,
		}).WithFields(
			version.RevisionOrderNumberKey,
			version.RevisionKey,
			version.MessageKey,
			version.CreateTimeKey,
		).Sort(sortVersions).Limit(versionsToFetch))

}

// Merge the specified slices of sibling task groups into a single slice.
func mergeGroups(groups ...[]siblingTaskGroup) []siblingTaskGroup {

	// get the total length of the groups
	totalLen := 0
	for _, group := range groups {
		totalLen += len(group)
	}

	// allocate the final slice
	mergedGroups := make([]siblingTaskGroup, totalLen)

	// add all the groups in to the merged one
	copied := 0
	for _, group := range groups {
		copy(mergedGroups[copied:], group)
		copied += len(group)
	}

	return mergedGroups
}

// Given a task name and a slice of versions, return the appropriate sibling
// groups of tasks.  They will be sorted by ascending revision order number,
// unless reverseOrder is true, in which case they will be sorted
// descending.
func getSiblingTaskGroups(displayName string, reverseOrder bool,
	versions []version.Version) ([]siblingTaskGroup, error) {

	orderNumbers := make([]int, 0, len(versions))
	for _, v := range versions {
		orderNumbers = append(orderNumbers, v.RevisionOrderNumber)
	}
	revisionCriteria := bson.M{"$in": orderNumbers}

	revisionSort := model.TaskRevisionOrderNumberKey
	if reverseOrder {
		revisionSort = "-" + revisionSort
	}

	tasks, err := model.FindAllTasks(
		bson.M{
			model.TaskRevisionOrderNumberKey: revisionCriteria,
			model.TaskDisplayNameKey:         displayName,
		},
		db.NoProjection,
		[]string{
			revisionSort,
		},
		db.NoSkip,
		db.NoLimit,
	)

	if err != nil {
		return nil, fmt.Errorf("error getting sibling tasks: %v", err)
	}

	return createSiblingTaskGroups(tasks, versions), nil

}

// Given versions and the appropriate tasks within them, sorted by build
// variant, create sibling groups for the tasks.
func createSiblingTaskGroups(tasks []model.Task, versions []version.Version) []siblingTaskGroup {

	// version id -> group
	groupsByVersion := map[string]siblingTaskGroup{}

	// create a group for each version
	for _, v := range versions {
		group := siblingTaskGroup{
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
		}

		// modify the status if the task is timed out or inactive
		if task.StatusDetails.TimedOut {
			if task.StatusDetails.TimeoutStage == "heartbeat" {
				blurb.Status = "heartbeat_timeout"
			} else {
				blurb.Status = "timed_out"
			}
		}
		if !task.Activated {
			blurb.Status = "inactive"
		}

		for _, result := range task.TestResults {
			if result.Status == mci.TestFailedStatus {
				blurb.Failures = append(blurb.Failures,
					result.TestFile)
			}
		}

		// add the blurb to the appropriate group
		groupForVersion := groupsByVersion[task.Version]
		groupForVersion.TaskBlurbs = append(groupForVersion.TaskBlurbs, blurb)
		groupsByVersion[task.Version] = groupForVersion

	}

	// create a slice of the sibling groups, in the appropriate order
	orderedGroups := make([]siblingTaskGroup, 0, len(versions))
	for _, version := range versions {
		orderedGroups = append(orderedGroups, groupsByVersion[version.Id])
	}

	return orderedGroups

}
