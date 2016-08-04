package service

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
)

const (
	// VersionItemsToCreate is the number of waterfall versions to create,
	// including rolled-up ones.
	VersionItemsToCreate = 5

	// SkipQueryParam is the string field for the skip value in the URL
	// (how many versions to skip).
	SkipQueryParam = "skip"

	InactiveStatus = "inactive"
)

// Pull the skip value out of the http request
func skipValue(r *http.Request) (int, error) {
	// determine how many versions to skip
	toSkipStr := r.FormValue(SkipQueryParam)
	if toSkipStr == "" {
		toSkipStr = "0"
	}
	return strconv.Atoi(toSkipStr)
}

// uiStatus determines task status label.
func uiStatus(task waterfallTask) string {
	switch task.Status {
	case "started":
		return "started"
	case "undispatched":
		if task.Activated {
			return "undispatched"
		} else {
			return "inactive"
		}
	case "success":
		return "success"
	case "failed":
		return "failed"
	case "dispatched":
		return "dispatched"
	default:
		return ""
	}
}

// waterfallData is all of the data that gets sent to the waterfall page on load
type waterfallData struct {
	Versions          []waterfallVersion `json:"versions"`
	BuildVariants     []string           `json:"build_variants"`
	TotalVersions     int                `json:"total_versions"`      // total number of versions (for pagination)
	CurrentSkip       int                `json:"current_skip"`        // number of versions skipped so far
	PreviousPageCount int                `json:"previous_page_count"` // number of versions on previous page
}

// Waterfall-specific representation of a single task
type waterfallTask struct {
	Id            string                  `json:"id"`
	Status        string                  `json:"status"`
	StatusDetails apimodels.TaskEndDetail `json:"task_end_details"`
	DisplayName   string                  `json:"display_name"`
	TimeTaken     time.Duration           `json:"time_taken"`
	Activated     bool                    `json:"activated"`
}

// Waterfall-specific representation of a single build. In the waterfall page, this data is used to render the
// intersection of one build variant and one version. waterfallBuild is null if the version is rolled up.
// Tasks holds an array of tasks that this build runs and the relevant data associated with each one
// Id is the unique id for each build
// BuildVariant stores the relevant data for the build variant of each build,
// including the unique id and human readable display name
// TaskStatusCount holds all the data needed for the collapsed view of a build
type waterfallBuild struct {
	Id              string                   `json:"id"`
	BuildVariant    waterfallBuildVariant    `json:"build_variant"`
	Tasks           []waterfallTask          `json:"tasks"`
	TaskStatusCount waterfallTaskStatusCount `json:"waterfallTaskStatusCount"`
}

// struct to hold all the numbers for different task types
// used by the front-end to show the rolled up view
type waterfallTaskStatusCount struct {
	Succeeded    int `json:"succeeded"`
	Failed       int `json:"failed"`
	Started      int `json:"started"`
	Undispatched int `json:"undispatched"`
	Inactive     int `json:"inactive"`
	Dispatched   int `json:"dispatched"`
	TimedOut     int `json:"timed_out"`
}

// waterfallBuildVariant stores the Id and DisplayName for a given build
// This struct is associated with one waterfallBuild
type waterfallBuildVariant struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Waterfall-specific representation of a single version element.  If the
// RolledUp field is false, then this contains information about a single
// version, and all of the metadata fields (ids, messages, etc) will each
// have length 1.  If the RolledUp field is true, then this version element
// actually contains information about a group of contiguous inactive versions.
// In that case, ids[x] will have the version id corresponding to messages[x]
// and authors[x] (as well for the other metadata fields).  The rolled-up
// versions will be backwards-ordered in time.
type waterfallVersion struct {

	// whether or not the version element actually consists of multiple inactive
	// versions rolled up into one
	RolledUp bool `json:"rolled_up"`

	// metadata about the enclosed versions.  if this version does not consist
	// of multiple rolled-up versions, these will each only have length 1
	Ids         []string    `json:"ids"`
	Messages    []string    `json:"messages"`
	Authors     []string    `json:"authors"`
	CreateTimes []time.Time `json:"create_times"`
	Revisions   []string    `json:"revisions"`

	// the builds that are a part of the version. this will be empty if the
	// element consists of multiple rolled-up versions
	Builds []waterfallBuild `json:"builds"`

	// used to hold any errors that were found in creating the version
	Errors   []waterfallVersionError `json:"errors"`
	Warnings []waterfallVersionError `json:"warnings"`
	Ignoreds []bool                  `json:"ignoreds"`
}

type waterfallVersionError struct {
	Messages []string `json:"messages"`
}

// Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func getVersionsAndVariants(skip int, numVersionElements int, project *model.Project) ([]waterfallVersion, []string, error) {
	// the final array of versions to return
	finalVersions := []waterfallVersion{}

	// keep track of the build variants we see
	bvSet := map[string]bool{}

	// build variant mappings - used so we can store the display name as
	// the build variant field of a build
	buildVariantMappings := project.GetVariantMappings()

	// keep track of the last rolled-up version, so inactive versions can
	// be added
	var lastRolledUpVersion *waterfallVersion = nil

	// loop until we have enough from the db
	for len(finalVersions) < numVersionElements {

		// fetch the versions and associated builds
		versionsFromDB, buildsByVersion, err :=
			fetchVersionsAndAssociatedBuilds(project, skip, numVersionElements)

		if err != nil {
			return nil, nil, fmt.Errorf("error fetching versions and builds:"+
				" %v", err)
		}

		// if we've reached the beginning of all versions
		if len(versionsFromDB) == 0 {
			break
		}

		// update the amount skipped
		skip += len(versionsFromDB)

		// create the necessary versions, rolling up inactive ones
		for _, version := range versionsFromDB {

			// if we have hit enough versions, break out
			if len(finalVersions) == numVersionElements {
				break
			}

			// the builds for the version
			buildsInVersion := buildsByVersion[version.Id]

			// see if there are any active tasks in the version
			versionActive := anyActiveTasks(buildsInVersion)

			// add any represented build variants to the set
			for _, build := range buildsInVersion {
				bvSet[build.BuildVariant] = true
			}

			// if it is inactive, roll up the version and don't create any
			// builds for it
			if !versionActive {
				if lastRolledUpVersion == nil {
					lastRolledUpVersion = &waterfallVersion{RolledUp: true}
				}

				// add the version metadata into the last rolled-up version
				lastRolledUpVersion.Ids = append(lastRolledUpVersion.Ids,
					version.Id)
				lastRolledUpVersion.Authors = append(lastRolledUpVersion.Authors,
					version.Author)
				lastRolledUpVersion.Errors = append(
					lastRolledUpVersion.Errors, waterfallVersionError{version.Errors})
				lastRolledUpVersion.Warnings = append(
					lastRolledUpVersion.Warnings, waterfallVersionError{version.Warnings})
				lastRolledUpVersion.Messages = append(
					lastRolledUpVersion.Messages, version.Message)
				lastRolledUpVersion.Ignoreds = append(
					lastRolledUpVersion.Ignoreds, version.Ignored)
				lastRolledUpVersion.CreateTimes = append(
					lastRolledUpVersion.CreateTimes, version.CreateTime)
				lastRolledUpVersion.Revisions = append(
					lastRolledUpVersion.Revisions, version.Revision)

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
				Ids:         []string{version.Id},
				Messages:    []string{version.Message},
				Authors:     []string{version.Author},
				CreateTimes: []time.Time{version.CreateTime},
				Revisions:   []string{version.Revision},
				Errors:      []waterfallVersionError{{version.Errors}},
				Warnings:    []waterfallVersionError{{version.Warnings}},
				Ignoreds:    []bool{version.Ignored},
			}

			// add the builds to the version
			for _, build := range buildsInVersion {

				buildVariant := waterfallBuildVariant{
					Id:          build.BuildVariant,
					DisplayName: buildVariantMappings[build.BuildVariant],
				}

				buildForWaterfall := waterfallBuild{
					Id:           build.Id,
					BuildVariant: buildVariant,
				}

				if buildForWaterfall.BuildVariant.DisplayName == "" {
					buildForWaterfall.BuildVariant.DisplayName = build.BuildVariant +
						" (removed)"
				}

				//initialize and set TaskStatusCount fields to zero
				statusCount := waterfallTaskStatusCount{}

				// add the tasks to the build
				for _, task := range build.Tasks {
					taskForWaterfall := waterfallTask{
						Id:            task.Id,
						Status:        task.Status,
						StatusDetails: task.StatusDetails,
						DisplayName:   task.DisplayName,
						Activated:     task.Activated,
						TimeTaken:     task.TimeTaken,
					}
					taskForWaterfall.Status = uiStatus(taskForWaterfall)

					switch taskForWaterfall.Status {
					case "success":
						statusCount.Succeeded++
					case "failed":
						if taskForWaterfall.StatusDetails.TimedOut && taskForWaterfall.StatusDetails.Description == "heartbeat" {
							statusCount.TimedOut++
						} else {
							statusCount.Failed++
						}
					case "started":
						statusCount.Started++
					case "undispatched":
						statusCount.Undispatched++
					case "dispatched":
						statusCount.Started++
					case "inactive":
						statusCount.Inactive++
					}
					// if the task is inactive, set its status to inactive
					if !task.Activated {
						taskForWaterfall.Status = InactiveStatus
					}

					buildForWaterfall.Tasks = append(buildForWaterfall.Tasks, taskForWaterfall)
				}

				buildForWaterfall.TaskStatusCount = statusCount

				activeVersion.Builds =
					append(activeVersion.Builds, buildForWaterfall)
			}

			// add the version
			finalVersions = append(finalVersions, activeVersion)

		}
	}

	// if the last version was rolled-up, add it
	if lastRolledUpVersion != nil {
		finalVersions = append(finalVersions, *lastRolledUpVersion)
	}

	// create the list of display names for the build variants represented
	buildVariants := []string{}
	for name := range bvSet {
		displayName := buildVariantMappings[name]
		if displayName == "" {
			displayName = name + " (removed)"
		}
		buildVariants = append(buildVariants, displayName)
	}

	return finalVersions, buildVariants, nil
}

// Helper function to fetch a group of versions and their associated builds.
// Returns the versions themselves, as well as a map of version id -> the
// builds that are a part of the version (unsorted).
func fetchVersionsAndAssociatedBuilds(project *model.Project, skip int, numVersions int) ([]version.Version, map[string][]build.Build, error) {

	// fetch the versions from the db
	versionsFromDB, err := version.Find(version.ByProjectId(project.Identifier).
		WithFields(
			version.RevisionKey,
			version.ErrorsKey,
			version.WarningsKey,
			version.IgnoredKey,
			version.MessageKey,
			version.AuthorKey,
			version.RevisionOrderNumberKey,
			version.CreateTimeKey,
		).Sort([]string{"-" + version.RevisionOrderNumberKey}).Skip(skip).Limit(numVersions))

	if err != nil {
		return nil, nil, fmt.Errorf("error fetching versions from database: %v", err)
	}

	// create a slice of the version ids (used to fetch the builds)
	versionIds := make([]string, 0, len(versionsFromDB))
	for _, v := range versionsFromDB {
		versionIds = append(versionIds, v.Id)
	}

	// fetch all of the builds (with only relevant fields)
	buildsFromDb, err := build.Find(
		build.ByVersions(versionIds).
			WithFields(build.BuildVariantKey, build.TasksKey, build.VersionKey))
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching builds from database: %v", err)
	}

	// sort the builds by version
	buildsByVersion := map[string][]build.Build{}
	for _, build := range buildsFromDb {
		buildsByVersion[build.Version] = append(
			buildsByVersion[build.Version], build)
	}

	return versionsFromDB, buildsByVersion, nil
}

// Takes in a slice of tasks, and determines whether any of the tasks in
// any of the builds are active.
func anyActiveTasks(builds []build.Build) bool {
	for _, build := range builds {
		for _, task := range build.Tasks {
			if task.Activated {
				return true
			}
		}
	}
	return false
}

// Calculates how many actual versions would appear on the previous page, given
// the starting skip for the current page as well as the number of version
// elements per page (including elements containing rolled-up versions).
func countOnPreviousPage(skip int, numVersionElements int,
	project *model.Project) (int, error) {

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
			fetchVersionsAndAssociatedBuilds(project, stepBack, toFetch)

		if err != nil {
			return 0, fmt.Errorf("error fetching versions and builds: %v", err)
		}

		// for each of the versions fetched (iterating backwards), calculate
		// how much it contributes to the version elements that would be
		// created
		for i := len(versionsFromDB) - 1; i >= 0; i-- {

			// increment the versions we've fetched
			versionsFetched += 1
			// if there are any active tasks
			if anyActiveTasks(buildsByVersion[versionsFromDB[i].Id]) {

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

// Create and return the waterfall data we need to render the page.
// Http handler for the waterfall page
func (uis *UIServer) waterfallPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	skip, err := skipValue(r)
	if err != nil {
		skip = 0
	}

	finalData := waterfallData{}

	// first, get all of the versions and variants we will need
	finalData.Versions, finalData.BuildVariants, err = getVersionsAndVariants(skip,
		VersionItemsToCreate, projCtx.Project)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// compute the total number of versions that exist
	finalData.TotalVersions, err = version.Count(version.ByProjectId(projCtx.Project.Identifier))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// compute the number of versions on the previous page
	finalData.PreviousPageCount, err = countOnPreviousPage(skip, VersionItemsToCreate, projCtx.Project)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// add in the skip value
	finalData.CurrentSkip = skip

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Data        waterfallData
	}{projCtx, GetUser(r), finalData}, "base", "waterfall.html", "base_angular.html", "menu.html")
}
