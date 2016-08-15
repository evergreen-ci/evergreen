package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
)

type waterfallDataOld struct {
	Versions          []waterfallVersionOld `json:"versions"`
	BuildVariants     []string              `json:"build_variants"`
	TotalVersions     int                   `json:"total_versions"`      // total number of versions (for pagination)
	CurrentSkip       int                   `json:"current_skip"`        // number of versions skipped so far
	PreviousPageCount int                   `json:"previous_page_count"` // number of versions on previous page
}

// Waterfall-specific representation of a single task
type waterfallTaskOld struct {
	Id            string                  `json:"id"`
	Status        string                  `json:"status"`
	StatusDetails apimodels.TaskEndDetail `json:"task_end_details"`
	DisplayName   string                  `json:"display_name"`
	TimeTaken     time.Duration           `json:"time_taken"`
	Activated     bool                    `json:"activated"`
}

// Waterfall-specific representation of a single build
type waterfallBuildOld struct {
	Id           string             `json:"id"`
	BuildVariant string             `json:"build_variant"`
	Tasks        []waterfallTaskOld `json:"tasks"`
}

// Waterfall-specific representation of a single version element.  If the
// RolledUp field is false, then this contains information about a single
// version, and all of the metadata fields (ids, messages, etc) will each
// have length 1.  If the RolledUp field is true, then this version element
// actually contains information about a group of contiguous inactive versions.
// In that case, ids[x] will have the version id corresponding to messages[x]
// and authors[x] (as well for the other metadata fields).  The rolled-up
// versions will be backwards-ordered in time.
type waterfallVersionOld struct {

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
	Builds []waterfallBuildOld `json:"builds"`

	// used to hold any errors that were found in creating the version
	Errors   []waterfallVersionError `json:"errors"`
	Warnings []waterfallVersionError `json:"warnings"`
	Ignoreds []bool                  `json:"ignoreds"`
}

// Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func getVersionsAndVariantsOld(skip int, numVersionElements int, project *model.Project) ([]waterfallVersionOld, []string, error) {
	// the final array of versions to return
	finalVersions := []waterfallVersionOld{}

	// keep track of the build variants we see
	bvSet := map[string]bool{}

	// build variant mappings - used so we can store the display name as
	// the build variant field of a build
	buildVariantMappings := project.GetVariantMappings()

	// keep track of the last rolled-up version, so inactive versions can
	// be added
	var lastRolledUpVersion *waterfallVersionOld = nil

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
					lastRolledUpVersion = &waterfallVersionOld{RolledUp: true}
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
			activeVersion := waterfallVersionOld{
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

				buildForWaterfall := waterfallBuildOld{
					Id:           build.Id,
					BuildVariant: buildVariantMappings[build.BuildVariant],
				}

				if buildForWaterfall.BuildVariant == "" {
					buildForWaterfall.BuildVariant = build.BuildVariant +
						" (removed)"
				}

				// add the tasks to the build
				for _, task := range build.Tasks {
					taskForWaterfall := waterfallTaskOld{
						Id:            task.Id,
						Status:        task.Status,
						StatusDetails: task.StatusDetails,
						DisplayName:   task.DisplayName,
						Activated:     task.Activated,
						TimeTaken:     task.TimeTaken,
					}

					// if the task is inactive, set its status to inactive
					if !task.Activated {
						taskForWaterfall.Status = InactiveStatus
					}

					buildForWaterfall.Tasks = append(buildForWaterfall.Tasks, taskForWaterfall)
				}

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

// Create and return the waterfall data we need to render the page.
// Http handler for the waterfall page
func (uis *UIServer) waterfallPageOld(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Project == nil {
		uis.ProjectNotFound(projCtx, w, r)
		return
	}

	skip, err := skipValue(r)
	if err != nil {
		skip = 0
	}

	finalData := waterfallDataOld{}

	// first, get all of the versions and variants we will need
	finalData.Versions, finalData.BuildVariants, err = getVersionsAndVariantsOld(skip,
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
		Data        waterfallDataOld
	}{projCtx, GetUser(r), finalData}, "base", "waterfall_old.html", "base_angular.html", "menu.html")
}
