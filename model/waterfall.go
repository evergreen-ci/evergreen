package model

import (
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
)

type VersionVariantData struct {
	Rows          map[string]WaterfallRow `json:"rows"`
	Versions      []WaterfallVersion      `json:"versions"`
	BuildVariants waterfallBuildVariants  `json:"build_variants"`
}

// waterfallData is all of the data that gets sent to the waterfall page on load
type WaterfallData struct {
	Rows              []WaterfallRow     `json:"rows"`
	Versions          []WaterfallVersion `json:"versions"`
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

// WaterfallRow represents one row associated with a build variant.
type WaterfallRow struct {
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
	FailedTestNames  []string                `json:"failed_test_names,omitempty"`
	ExpectedDuration time.Duration           `json:"expected_duration,omitempty"`
	StartTime        int64                   `json:"start_time"`
}

// WaterfallVersion holds the waterfall UI representation of a single version (column)
// If the RolledUp field is false, then it contains information about
// a single version and the metadata fields will be of length 1.
// If the RolledUp field is true, this represents multiple inactive versions, with each element
// in the metadata arrays corresponding to one inactive version,
// ordered from most recent inactive version to earliest.
type WaterfallVersion struct {

	// whether or not the version element actually consists of multiple inactive
	// versions rolled up into one
	RolledUp bool `json:"rolled_up"`

	// metadata about the enclosed versions.  if this version does not consist
	// of multiple rolled-up versions, these will each only have length 1
	Ids                 []string    `json:"ids"`
	Messages            []string    `json:"messages"`
	Authors             []string    `json:"authors"`
	CreateTimes         []time.Time `json:"create_times"`
	Revisions           []string    `json:"revisions"`
	RevisionOrderNumber int         `json:"revision_order"`

	// used to hold any errors that were found in creating the version
	Errors   []waterfallVersionError `json:"errors"`
	Warnings []waterfallVersionError `json:"warnings"`
	Ignoreds []bool                  `json:"ignoreds"`
}

// waterfallVersions implements the sort interface to allow backend sorting.
type waterfallVersions []WaterfallVersion

func (wfv waterfallVersions) Len() int {
	return len(wfv)
}

func (wfv waterfallVersions) Less(i, j int) bool {
	return wfv[i].RevisionOrderNumber > wfv[j].RevisionOrderNumber
}

func (wfv waterfallVersions) Swap(i, j int) {
	wfv[i], wfv[j] = wfv[j], wfv[i]
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

// For given build variant, variant display name and variant search query
// checks if matched variant has active tasks
func variantHasActiveTasks(b build.Build, bvDisplayName string, variantQuery string) bool {
	return strings.Contains(
		strings.ToUpper(bvDisplayName), strings.ToUpper(variantQuery),
	) && b.IsActive()
}

// Takes in a slice of tasks, and determines whether any of the tasks in
// any of the builds are active.
func anyActiveTasks(builds []build.Build) bool {
	for _, build := range builds {
		return build.IsActive()
	}
	return false
}

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

// createWaterfallTasks takes ina  build's task cache returns a list of waterfallTasks.
func createWaterfallTasks(tasks []build.TaskCache) ([]waterfallTask, task.TaskStatusCount) {
	//initialize and set TaskStatusCount fields to zero
	statusCount := task.TaskStatusCount{}
	waterfallTasks := []waterfallTask{}

	// add the tasks to the build
	for _, t := range tasks {
		taskForWaterfall := waterfallTask{
			Id:            t.Id,
			Status:        t.Status,
			StatusDetails: t.StatusDetails,
			DisplayName:   t.DisplayName,
			Activated:     t.Activated,
			TimeTaken:     t.TimeTaken,
			StartTime:     t.StartTime.UnixNano(),
		}
		taskForWaterfall.Status = uiStatus(taskForWaterfall)

		statusCount.IncrementStatus(taskForWaterfall.Status, taskForWaterfall.StatusDetails)

		waterfallTasks = append(waterfallTasks, taskForWaterfall)
	}
	return waterfallTasks, statusCount
}

// addFailedTests adds all of the failed tests associated with a task to its entry in the WaterfallRow.
// addFailedAndStartedTests adds all of the failed tests associated with a task to its entry in the waterfallRow
// and adds the estimated duration to started tasks.
func addFailedAndStartedTests(waterfallRows map[string]WaterfallRow, failedAndStartedTasks []task.Task) {
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

// Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func GetWaterfallVersionsAndVariants(
	skip,
	numVersionElements int,
	project *Project,
	variantQuery string,
) (VersionVariantData, error) {
	// the final array of versions to return
	finalVersions := []WaterfallVersion{}

	// keep track of the build variants we see
	bvSet := map[string]bool{}

	waterfallRows := map[string]WaterfallRow{}

	// build variant mappings - used so we can store the display name as
	// the build variant field of a build
	buildVariantMappings := project.GetVariantMappings()

	// keep track of the last rolled-up version, so inactive versions can
	// be added
	var lastRolledUpVersion *WaterfallVersion

	// loop until we have enough from the db
	for len(finalVersions) < numVersionElements {

		// fetch the versions and associated builds
		versionsFromDB, buildsByVersion, err :=
			FetchVersionsAndAssociatedBuilds(project, skip, numVersionElements)

		if err != nil {
			return VersionVariantData{}, errors.Wrap(err,
				"error fetching versions and builds:")
		}

		// if we've reached the beginning of all versions
		if len(versionsFromDB) == 0 {
			break
		}

		// to fetch started tasks and failed tests for providing additional context
		// in a tooltip
		failedAndStartedTaskIds := []string{}

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
			versionActive := anyActiveTasks(buildsInVersion)

			variantMatched := false

			// add any represented build variants to the set and initialize rows
			for _, b := range buildsInVersion {
				bvSet[b.BuildVariant] = true

				buildVariant := waterfallBuildVariant{
					Id:          b.BuildVariant,
					DisplayName: buildVariantMappings[b.BuildVariant],
				}

				if buildVariant.DisplayName == "" {
					buildVariant.DisplayName = b.BuildVariant +
						" (removed)"
				}

				// The version is marked active if there are any
				// activated tasks for the varant
				if variantQuery != "" {
					if versionActive && !variantMatched {
						variantMatched = variantHasActiveTasks(
							b, buildVariant.DisplayName, variantQuery,
						)
					}
				} else {
					variantMatched = true
				}

				if _, ok := waterfallRows[b.BuildVariant]; !ok {
					waterfallRows[b.BuildVariant] = WaterfallRow{
						Builds:       map[string]waterfallBuild{},
						BuildVariant: buildVariant,
					}
				}

			}

			// if it is inactive, roll up the version and don't create any
			// builds for it
			if !versionActive || !variantMatched {
				if lastRolledUpVersion == nil {
					lastRolledUpVersion = &WaterfallVersion{RolledUp: true, RevisionOrderNumber: versionFromDB.RevisionOrderNumber}
				}

				// add the version metadata into the last rolled-up version
				lastRolledUpVersion.Ids = append(lastRolledUpVersion.Ids,
					versionFromDB.Id)
				lastRolledUpVersion.Authors = append(lastRolledUpVersion.Authors,
					versionFromDB.Author)
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
			activeVersion := WaterfallVersion{
				Ids:                 []string{versionFromDB.Id},
				Messages:            []string{versionFromDB.Message},
				Authors:             []string{versionFromDB.Author},
				CreateTimes:         []time.Time{versionFromDB.CreateTime},
				Revisions:           []string{versionFromDB.Revision},
				Errors:              []waterfallVersionError{{versionFromDB.Errors}},
				Warnings:            []waterfallVersionError{{versionFromDB.Warnings}},
				Ignoreds:            []bool{versionFromDB.Ignored},
				RevisionOrderNumber: versionFromDB.RevisionOrderNumber,
			}

			// add the builds to the waterfall row
			for _, b := range buildsInVersion {
				currentRow := waterfallRows[b.BuildVariant]
				buildForWaterfall := waterfallBuild{
					Id:      b.Id,
					Version: versionFromDB.Id,
				}

				tasks, statusCount := createWaterfallTasks(b.Tasks)
				buildForWaterfall.Tasks = tasks
				buildForWaterfall.TaskStatusCount = statusCount
				currentRow.Builds[versionFromDB.Id] = buildForWaterfall
				waterfallRows[b.BuildVariant] = currentRow
				for _, task := range buildForWaterfall.Tasks {
					if task.Status == evergreen.TaskFailed || task.Status == evergreen.TaskStarted {
						failedAndStartedTaskIds = append(failedAndStartedTaskIds, task.Id)
					}
				}
			}

			// add the version
			finalVersions = append(finalVersions, activeVersion)

		}

		failedAndStartedTasks, err := task.Find(task.ByIds(failedAndStartedTaskIds))
		if err != nil {
			return VersionVariantData{}, errors.Wrap(err, "error fetching failed tasks")

		}

		for i := range failedAndStartedTasks {
			if err := failedAndStartedTasks[i].MergeNewTestResults(); err != nil {
				return VersionVariantData{}, errors.Wrap(err, "error merging test results")
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
			displayName = name + " (removed)"
		}
		buildVariants = append(buildVariants, waterfallBuildVariant{Id: name, DisplayName: displayName})
	}

	return VersionVariantData{
		Rows:          waterfallRows,
		Versions:      finalVersions,
		BuildVariants: buildVariants,
	}, nil
}

func WaterfallDataAdaptor(
	vvData VersionVariantData,
	project *Project,
	skip int,
	limit int,
	variantQuery string,
) (WaterfallData, error) {
	var err error
	finalData := WaterfallData{}
	var wfv waterfallVersions = vvData.Versions

	sort.Sort(wfv)
	finalData.Versions = wfv

	sort.Sort(vvData.BuildVariants)
	rows := []WaterfallRow{}
	for _, bv := range vvData.BuildVariants {
		rows = append(rows, vvData.Rows[bv.Id])
	}
	finalData.Rows = rows

	// compute the total number of versions that exist
	finalData.TotalVersions, err = version.Count(version.ByProjectId(project.Identifier))
	if err != nil {
		return WaterfallData{}, err
	}

	// compute the number of versions on the previous page
	finalData.PreviousPageCount, err = countOnPreviousPage(skip, limit, project, variantQuery)
	if err != nil {
		return WaterfallData{}, err
	}

	// add in the skip value
	finalData.CurrentSkip = skip

	// pass it the current time
	finalData.CurrentTime = time.Now().UnixNano()

	return finalData, nil
}

// Calculates how many actual versions would appear on the previous page, given
// the starting skip for the current page as well as the number of version
// elements per page (including elements containing rolled-up versions).
func countOnPreviousPage(
	skip int,
	numVersionElements int,
	project *Project,
	variantQuery string,
) (int, error) {

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
		versionsFromDB, buildsByVersion, err :=
			FetchVersionsAndAssociatedBuilds(project, stepBack, toFetch)

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

			if anyActiveTasks(buildsInVersion) {

				for _, b := range buildsInVersion {
					bvDisplayName := buildVariantMappings[b.BuildVariant]

					if bvDisplayName == "" {
						bvDisplayName = b.BuildVariant + " (removed)"
					}

					// When versions is active and variane query matches
					// variant display name, mark the version as inactive
					if variantQuery != "" {
						if !variantMatched {
							variantMatched = variantHasActiveTasks(
								b, bvDisplayName, variantQuery,
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
