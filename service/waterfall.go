package service

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

const (
	waterfallPerPageLimit  = 5
	waterfallBVFilterParam = "bv_filter"
	waterfallSkipParam     = "skip"
	showUpstreamParam      = "upstream"
	maxTestResultCalls     = 500
)

// uiStatus determines task status label.
func uiStatus(task waterfallTask) string {
	switch task.Status {
	case evergreen.TaskStarted, evergreen.TaskSucceeded,
		evergreen.TaskFailed, evergreen.TaskDispatched:
		return task.Status
	case evergreen.TaskUndispatched:
		if task.Activated {
			return evergreen.TaskUndispatched
		} else {
			return evergreen.TaskInactive
		}
	default:
		return task.Status
	}
}

type versionVariantData struct {
	Rows          map[string]waterfallRow `json:"rows"`
	Versions      []waterfallVersion      `json:"versions"`
	BuildVariants waterfallBuildVariants  `json:"build_variants"`
}

// waterfallData is all of the data that gets sent to the waterfall page on load
type waterfallData struct {
	Rows              []waterfallRow     `json:"rows"`
	Versions          []waterfallVersion `json:"versions"`
	TotalVersions     int                `json:"total_versions"`      // total number of versions (for pagination)
	CurrentSkip       int                `json:"current_skip"`        // number of versions skipped so far
	PreviousPageCount int                `json:"previous_page_count"` // number of versions on previous page
	CurrentTime       int64              `json:"current_time"`        // time used to calculate the eta of started task
}

// waterfallBuildVariant stores the Id and DisplayName for a given build
// This struct is associated with one waterfallBuild
type waterfallBuildVariant struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// waterfallRow represents one row associated with a build variant.
type waterfallRow struct {
	BuildVariant waterfallBuildVariant     `json:"build_variant"`
	Builds       map[string]waterfallBuild `json:"builds"`
}

// waterfallBuild represents one set of tests for a given build variant and version
type waterfallBuild struct {
	Id              string               `json:"id"`
	Active          bool                 `json:"active"`
	Version         string               `json:"version"`
	Tasks           []waterfallTask      `json:"tasks"`
	TaskStatusCount task.TaskStatusCount `json:"taskStatusCount"`
}

// waterfallTask represents one task in the waterfall UI.
type waterfallTask struct {
	Id               string                  `json:"id"`
	Status           string                  `json:"status"`
	StatusDetails    apimodels.TaskEndDetail `json:"task_end_details"`
	DisplayName      string                  `json:"display_name"`
	TimeTaken        time.Duration           `json:"time_taken"`
	Activated        bool                    `json:"activated"`
	Blocked          bool                    `json:"blocked"`
	FailedTestNames  []string                `json:"failed_test_names,omitempty"`
	ExpectedDuration time.Duration           `json:"expected_duration,omitempty"`
	StartTime        int64                   `json:"start_time"`
}

// failedTest holds all the information for displaying context about tests that failed in a
// waterfall page tooltip.

// waterfallVersion holds the waterfall UI representation of a single version (column)
// If the RolledUp field is false, then it contains information about
// a single version and the metadata fields will be of length 1.
// If the RolledUp field is true, this represents multiple inactive versions, with each element
// in the metadata arrays corresponding to one inactive version,
// ordered from most recent inactive version to earliest.
type waterfallVersion struct {

	// whether or not the version element actually consists of multiple inactive
	// versions rolled up into one
	RolledUp bool `json:"rolled_up"`

	// metadata about the enclosed versions.  if this version does not consist
	// of multiple rolled-up versions, these will each only have length 1
	Ids                 []string         `json:"ids"`
	Messages            []string         `json:"messages"`
	Authors             []string         `json:"authors"`
	CreateTimes         []time.Time      `json:"create_times"`
	Revisions           []string         `json:"revisions"`
	RevisionOrderNumber int              `json:"revision_order"`
	UpstreamData        []uiUpstreamData `json:"upstream_data"`
	GitTags             []string         `json:"git_tags"`

	// used to hold any errors that were found in creating the version
	Errors   []waterfallVersionError `json:"errors"`
	Warnings []waterfallVersionError `json:"warnings"`
	Ignoreds []bool                  `json:"ignoreds"`
}

type waterfallVersionError struct {
	Messages []string `json:"messages"`
}

// waterfallBuildVariants implements the sort interface to allow backend sorting.
type waterfallBuildVariants []waterfallBuildVariant

func (wfbv waterfallBuildVariants) Len() int {
	return len(wfbv)
}

func (wfbv waterfallBuildVariants) Less(i, j int) bool {
	return wfbv[i].DisplayName < wfbv[j].DisplayName
}

func (wfbv waterfallBuildVariants) Swap(i, j int) {
	wfbv[i], wfbv[j] = wfbv[j], wfbv[i]
}

// waterfallVersions implements the sort interface to allow backend sorting.
type waterfallVersions []waterfallVersion

func (wfv waterfallVersions) Len() int {
	return len(wfv)
}

func (wfv waterfallVersions) Less(i, j int) bool {
	return wfv[i].RevisionOrderNumber > wfv[j].RevisionOrderNumber
}

func (wfv waterfallVersions) Swap(i, j int) {
	wfv[i], wfv[j] = wfv[j], wfv[i]
}

// createWaterfallTasks takes in a list of tasks returns a list of waterfallTasks.
func createWaterfallTasks(tasks []task.Task) ([]waterfallTask, task.TaskStatusCount) {
	//initialize and set TaskStatusCount fields to zero
	statusCount := task.TaskStatusCount{}
	waterfallTasks := []waterfallTask{}

	// add the tasks to the build
	for _, t := range tasks {
		taskForWaterfall := waterfallTask{
			Id:            t.Id,
			Status:        t.Status,
			StatusDetails: t.Details,
			DisplayName:   t.DisplayName,
			Activated:     t.Activated,
			Blocked:       t.Blocked(),
			TimeTaken:     t.TimeTaken,
			StartTime:     t.StartTime.UnixNano(),
		}
		taskForWaterfall.Status = uiStatus(taskForWaterfall)

		statusCount.IncrementStatus(taskForWaterfall.Status, taskForWaterfall.StatusDetails)

		waterfallTasks = append(waterfallTasks, taskForWaterfall)
	}
	return waterfallTasks, statusCount
}

// For given build variant, variant display name and variant search query
// checks if matched variant has active tasks
func variantHasActiveTasks(bvDisplayName, variantQuery string, tasks []task.Task) bool {
	return strings.Contains(
		strings.ToUpper(bvDisplayName), strings.ToUpper(variantQuery),
	) && task.AnyActiveTasks(tasks)
}

// Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func getVersionsAndVariants(skip, numVersionElements int, project *model.Project, variantQuery string, showTriggered bool) (versionVariantData, error) {
	// the final array of versions to return
	finalVersions := []waterfallVersion{}

	// keep track of the build variants we see
	bvSet := map[string]bool{}

	waterfallRows := map[string]waterfallRow{}

	// build variant mappings - used so we can store the display name as
	// the build variant field of a build
	buildVariantMappings := project.GetVariantMappings()

	// keep track of the last rolled-up version, so inactive versions can
	// be added
	var lastRolledUpVersion *waterfallVersion

	numTestResultCalls := 0
	// loop until we have enough from the db
	for len(finalVersions) < numVersionElements {

		// fetch the versions and associated builds
		versionsFromDB, buildsByVersion, tasksByBuild, err :=
			model.FetchVersionsBuildsAndTasks(project, skip, numVersionElements, showTriggered)

		if err != nil {
			return versionVariantData{}, errors.Wrap(err,
				"error fetching versions and builds:")
		}

		// if we've reached the beginning of all versions
		if len(versionsFromDB) == 0 {
			break
		}

		// update the amount skipped
		skip += len(versionsFromDB)

		// create the necessary versions, rolling up inactive ones
		for _, versionFromDB := range versionsFromDB {

			// if we have hit enough versions, break out
			if len(finalVersions) == numVersionElements {
				break
			}

			// the builds for the version
			buildsInVersion := buildsByVersion[versionFromDB.Id]

			// see if there are any active tasks in the version
			versionActive := false
			for _, b := range buildsInVersion {
				if task.AnyActiveTasks(tasksByBuild[b.Id]) {
					versionActive = true
				}
			}

			variantMatched := false

			// add any represented build variants to the set and initialize rows
			for _, b := range buildsInVersion {
				bvSet[b.BuildVariant] = true

				// variant may not be defined in project, in which case add display name to mapping
				if buildVariantMappings[b.BuildVariant] == "" {
					buildVariantMappings[b.BuildVariant] = b.DisplayName
				}
				buildVariant := waterfallBuildVariant{
					Id:          b.BuildVariant,
					DisplayName: buildVariantMappings[b.BuildVariant],
				}

				// The version is marked active if there are any
				// activated tasks for the variant
				if variantQuery != "" {
					if versionActive && !variantMatched {
						variantMatched = variantHasActiveTasks(
							buildVariant.DisplayName, variantQuery, tasksByBuild[b.Id],
						)
					}
				} else {
					variantMatched = true
				}

				if _, ok := waterfallRows[b.BuildVariant]; !ok {
					waterfallRows[b.BuildVariant] = waterfallRow{
						Builds:       map[string]waterfallBuild{},
						BuildVariant: buildVariant,
					}
				}

			}

			// if it is inactive, roll up the version and don't create any
			// builds for it
			if !versionActive || !variantMatched {
				if lastRolledUpVersion == nil {
					lastRolledUpVersion = &waterfallVersion{RolledUp: true, RevisionOrderNumber: versionFromDB.RevisionOrderNumber}
				}

				// add the version metadata into the last rolled-up version
				lastRolledUpVersion.Ids = append(lastRolledUpVersion.Ids, versionFromDB.Id)
				lastRolledUpVersion.Authors = append(lastRolledUpVersion.Authors, versionFromDB.Author)
				lastRolledUpVersion.Errors = append(
					lastRolledUpVersion.Errors, waterfallVersionError{versionFromDB.Errors})
				lastRolledUpVersion.Warnings = append(
					lastRolledUpVersion.Warnings, waterfallVersionError{versionFromDB.Warnings})
				lastRolledUpVersion.Messages = append(
					lastRolledUpVersion.Messages, versionFromDB.Message)
				lastRolledUpVersion.Ignoreds = append(
					lastRolledUpVersion.Ignoreds, versionFromDB.Ignored)
				lastRolledUpVersion.CreateTimes = append(
					lastRolledUpVersion.CreateTimes, versionFromDB.CreateTime)
				lastRolledUpVersion.Revisions = append(
					lastRolledUpVersion.Revisions, versionFromDB.Revision)
				lastRolledUpVersion.GitTags = append(
					lastRolledUpVersion.GitTags, model.GitTags(versionFromDB.GitTags).String())
				// move on to the next version
				continue
			}

			// add a pending rolled-up version, if it exists
			if lastRolledUpVersion != nil {
				finalVersions = append(finalVersions, *lastRolledUpVersion)
				lastRolledUpVersion = nil
			}

			// if we have hit enough versions, break out
			if len(finalVersions) == numVersionElements {
				break
			}

			// if the version can not be rolled up, create a fully fledged
			// version for it
			activeVersion := waterfallVersion{
				Ids:                 []string{versionFromDB.Id},
				Messages:            []string{versionFromDB.Message},
				Authors:             []string{versionFromDB.Author},
				CreateTimes:         []time.Time{versionFromDB.CreateTime},
				Revisions:           []string{versionFromDB.Revision},
				Errors:              []waterfallVersionError{{versionFromDB.Errors}},
				Warnings:            []waterfallVersionError{{versionFromDB.Warnings}},
				Ignoreds:            []bool{versionFromDB.Ignored},
				RevisionOrderNumber: versionFromDB.RevisionOrderNumber,
				GitTags:             []string{model.GitTags(versionFromDB.GitTags).String()},
			}
			if versionFromDB.TriggerID != "" {
				var projectName string
				projectName, err = model.GetUpstreamProjectName(versionFromDB.TriggerID, versionFromDB.TriggerType)
				if err != nil {
					return versionVariantData{}, err
				}
				activeVersion.UpstreamData = []uiUpstreamData{
					{ProjectName: projectName, TriggerID: versionFromDB.TriggerID, TriggerType: versionFromDB.TriggerType},
				}
			}

			// add the builds to the waterfall row
			for _, b := range buildsInVersion {
				currentRow := waterfallRows[b.BuildVariant]
				buildForWaterfall := waterfallBuild{
					Id:      b.Id,
					Version: versionFromDB.Id,
				}

				tasks, statusCount := createWaterfallTasks(tasksByBuild[b.Id])
				buildForWaterfall.Tasks = tasks
				buildForWaterfall.TaskStatusCount = statusCount
				currentRow.Builds[versionFromDB.Id] = buildForWaterfall
				waterfallRows[b.BuildVariant] = currentRow
			}

			// add the version
			finalVersions = append(finalVersions, activeVersion)

		}

		failedAndStartedTasks := []task.Task{}
		for _, tasks := range tasksByBuild {
			for _, t := range tasks {
				if t.Status == evergreen.TaskFailed || t.Status == evergreen.TaskStarted {
					// only call the legacy function if we need to, i.e. we aren't using cedar, we know there are legacy results,
					// or we don't know because we haven't cached this information yet.
					if !t.HasCedarResults && utility.FromBoolTPtr(t.HasLegacyResults) && numTestResultCalls < maxTestResultCalls {
						if err = t.PopulateTestResults(); err != nil {
							return versionVariantData{}, errors.Wrap(err, "populating test results")
						}
						numTestResultCalls++
					}
					failedAndStartedTasks = append(failedAndStartedTasks, t)
				}
			}
		}

		addFailedAndStartedTests(waterfallRows, failedAndStartedTasks)
	}

	// if the last version was rolled-up, add it
	if lastRolledUpVersion != nil {
		finalVersions = append(finalVersions, *lastRolledUpVersion)
	}

	// create the list of display names for the build variants represented
	buildVariants := waterfallBuildVariants{}
	for name := range bvSet {
		displayName := buildVariantMappings[name]
		if displayName == "" {
			displayName = name
		}
		buildVariants = append(buildVariants, waterfallBuildVariant{Id: name, DisplayName: displayName})
	}

	return versionVariantData{
		Rows:          waterfallRows,
		Versions:      finalVersions,
		BuildVariants: buildVariants,
	}, nil

}

// addFailedTests adds all of the failed tests associated with a task to its entry in the waterfallRow.
// addFailedAndStartedTests adds all of the failed tests associated with a task to its entry in the waterfallRow
// and adds the estimated duration to started tasks.
func addFailedAndStartedTests(waterfallRows map[string]waterfallRow, failedAndStartedTasks []task.Task) {
	failedTestsByTaskId := map[string][]string{}
	expectedDurationByTaskId := map[string]time.Duration{}
	for _, t := range failedAndStartedTasks {
		failedTests := []string{}
		for _, r := range t.LocalTestResults {
			if r.Status == evergreen.TestFailedStatus {
				failedTests = append(failedTests, r.TestFile)
			}
		}
		if t.Status == evergreen.TaskStarted {
			expectedDurationByTaskId[t.Id] = t.ExpectedDuration
		}
		failedTestsByTaskId[t.Id] = failedTests
	}
	for buildVariant, row := range waterfallRows {
		for versionId, build := range row.Builds {
			for i, task := range build.Tasks {
				if len(failedTestsByTaskId[task.Id]) != 0 {
					waterfallRows[buildVariant].Builds[versionId].Tasks[i].FailedTestNames = append(
						waterfallRows[buildVariant].Builds[versionId].Tasks[i].FailedTestNames,
						failedTestsByTaskId[task.Id]...)
					sort.Strings(waterfallRows[buildVariant].Builds[versionId].Tasks[i].FailedTestNames)
				}
				if duration, ok := expectedDurationByTaskId[task.Id]; ok {
					waterfallRows[buildVariant].Builds[versionId].Tasks[i].ExpectedDuration = duration
				}
			}
		}
	}
}

// Calculates how many actual versions would appear on the previous page, given
// the starting skip for the current page as well as the number of version
// elements per page (including elements containing rolled-up versions).
func countOnPreviousPage(skip int, numVersionElements int, project *model.Project, variantQuery string, showTriggered bool) (int, error) {
	buildVariantMappings := project.GetVariantMappings()

	// if there is no previous page
	if skip == 0 {
		return 0, nil
	}

	// the initial number of versions to be fetched per iteration
	toFetch := numVersionElements

	// the initial number of elements to step back from the current point
	// (capped to 0)
	stepBack := skip - numVersionElements
	if stepBack < 0 {
		toFetch = skip // only fetch up to the current point
		stepBack = 0
	}

	// bookkeeping: the number of version elements represented so far, as well
	// as the total number of versions fetched
	elementsCreated := 0
	versionsFetched := 0
	// bookkeeping: whether the previous version was active
	prevActive := true

	for {

		// fetch the versions and builds
		versionsFromDB, buildsByVersion, tasksByBuild, err :=
			model.FetchVersionsBuildsAndTasks(project, stepBack, toFetch, showTriggered)

		if err != nil {
			return 0, errors.Wrap(err, "error fetching versions and builds")
		}

		// for each of the versions fetched (iterating backwards), calculate
		// how much it contributes to the version elements that would be
		// created
		for i := len(versionsFromDB) - 1; i >= 0; i-- {

			versionFromDB := versionsFromDB[i]

			// increment the versions we've fetched
			versionsFetched += 1
			// if there are any active tasks
			buildsInVersion := buildsByVersion[versionFromDB.Id]

			variantMatched := false

			versionActive := false
			for _, b := range buildsInVersion {
				if task.AnyActiveTasks(tasksByBuild[b.Id]) {
					versionActive = true
				}
			}
			if versionActive {

				for _, b := range buildsInVersion {
					bvDisplayName := buildVariantMappings[b.BuildVariant]

					if bvDisplayName == "" {
						bvDisplayName = b.BuildVariant
					}

					// When versions is active and variane query matches
					// variant display name, mark the version as inactive
					if variantQuery != "" {
						if !variantMatched {
							variantMatched = variantHasActiveTasks(
								bvDisplayName, variantQuery, tasksByBuild[b.Id],
							)
						}
					} else {
						variantMatched = true
					}
				}

				// Skip versions which doen't match variant search query
				if !variantMatched {
					continue
				}

				// we may have stepped one over where the versions end, if
				// the last was inactive
				if elementsCreated == numVersionElements {
					return versionsFetched - 1, nil
				}

				// the active version would get its own element
				elementsCreated += 1
				prevActive = true

				// see if it's the last
				if elementsCreated == numVersionElements {
					return versionsFetched, nil
				}
			} else if prevActive {

				// only record a rolled-up version when we hit the first version
				// in it (walking backwards)
				elementsCreated += 1
				prevActive = false
			}

		}

		// if we've hit the most recent versions (can't step back farther)
		if stepBack == 0 {
			return versionsFetched, nil
		}

		// recalculate where to skip to and how many to fetch
		stepBack -= numVersionElements
		if stepBack < 0 {
			toFetch = stepBack + numVersionElements
			stepBack = 0
		}

	}
}

func waterfallDataAdaptor(vvData versionVariantData, project *model.Project, skip int, variantQuery string, showUpstream bool) (waterfallData, error) {
	var err error
	finalData := waterfallData{}
	var wfv waterfallVersions = vvData.Versions

	sort.Sort(wfv)
	finalData.Versions = wfv

	sort.Sort(vvData.BuildVariants)
	rows := []waterfallRow{}
	for _, bv := range vvData.BuildVariants {
		rows = append(rows, vvData.Rows[bv.Id])
	}
	finalData.Rows = rows

	// compute the total number of versions that exist
	finalData.TotalVersions, err = model.VersionCount(model.VersionByProjectId(project.Identifier))
	if err != nil {
		return waterfallData{}, err
	}

	// compute the number of versions on the previous page
	finalData.PreviousPageCount, err = countOnPreviousPage(skip, waterfallPerPageLimit, project, variantQuery, showUpstream)
	if err != nil {
		return waterfallData{}, err
	}

	// add in the skip value
	finalData.CurrentSkip = skip

	// pass it the current time
	finalData.CurrentTime = time.Now().UnixNano()

	return finalData, nil
}

// Create and return the waterfall data we need to render the page.
// Http handler for the waterfall page
func (uis *UIServer) waterfallPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		uis.ProjectNotFound(w, r)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		JiraHost string
		ViewData
	}{uis.Settings.Jira.Host, uis.GetCommonViewData(w, r, false, true)}, "base", "waterfall.html", "base_angular.html", "menu.html")
}

func (restapi restAPI) getWaterfallData(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	project, err := projCtx.GetProject()
	if err != nil || project == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding project"})
		return
	}

	query := r.URL.Query()

	skipQ := query.Get(waterfallSkipParam)
	skip := 0

	if skipQ != "" {
		skip, err = strconv.Atoi(skipQ)
		if err != nil {
			gimlet.WriteJSONResponse(
				w, http.StatusNotFound, responseError{Message: errors.Wrapf(
					err, "Invalid 'skip' value '%s'", skipQ).Error()},
			)
			return
		}
	}

	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil {
		limit = waterfallPerPageLimit
	}

	variantQuery := strings.TrimSpace(query.Get(waterfallBVFilterParam))

	showUpstream := (query.Get(showUpstreamParam) == "true")

	vvData, err := getVersionsAndVariants(skip, limit, project, variantQuery, showUpstream)
	if err != nil {
		gimlet.WriteJSONResponse(
			w, http.StatusNotFound, responseError{Message: errors.Wrap(
				err, "Error while loading versions and variants data").Error()},
		)
		return
	}

	finalData, err := waterfallDataAdaptor(vvData, project, skip, variantQuery, showUpstream)
	if err != nil {
		gimlet.WriteJSONResponse(
			w, http.StatusNotFound, responseError{Message: errors.Wrap(
				err, "Error while processing versions and variants data").Error()},
		)
		return
	}

	gimlet.WriteJSONResponse(w, http.StatusOK, finalData)
}
