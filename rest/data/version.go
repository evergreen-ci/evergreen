package data

import (
	"context"
	"net/http"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/repotracker"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBVersionConnector is a struct that implements Version related methods
// from the Connector through interactions with the backing database.
type DBVersionConnector struct{}

// GetProjectVersionsWithOptions returns the versions that fit the given constraint.
func GetProjectVersionsWithOptions(projectName string, opts model.GetVersionsOptions) ([]restModel.APIVersion, error) {
	versions, err := model.GetVersionsWithOptions(projectName, opts)
	if err != nil {
		return nil, err
	}
	res := []restModel.APIVersion{}
	for _, v := range versions {
		apiVersion := restModel.APIVersion{}
		apiVersion.BuildFromService(v)
		res = append(res, apiVersion)
	}
	return res, nil
}

// GetVersionsAndVariants Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func GetVersionsAndVariants(ctx context.Context, skip, numVersionElements int, project *model.Project) (*restModel.VersionVariantData, error) {
	// the final array of versions to return
	finalVersions := []restModel.APIVersions{}

	// list of builds that are in the search results, analogous to a row in the waterfall
	buildList := map[string]restModel.BuildList{}

	// buildvariant names that have at least 1 active version
	buildVariants := []string{}

	// build variant mappings - used so we can store the display name as
	// the build variant field of a build
	buildVariantMappings := project.GetVariantMappings()

	// keep track of the last rolled-up version, so inactive versions can
	// be added
	var lastRolledUpVersion *restModel.APIVersions

	// loop until we have enough from the db
	for len(finalVersions) < numVersionElements {

		// fetch the versions and associated builds
		versionsFromDB, buildsByVersion, tasksByBuild, err :=
			model.FetchVersionsBuildsAndTasks(project, skip, numVersionElements, true)

		if err != nil {
			return nil, errors.Wrap(err, "fetching versions and builds")
		}

		// if we've reached the beginning of all versions
		if len(versionsFromDB) == 0 {
			break
		}

		// to fetch started tasks and failed tests for providing additional context
		// in a tooltip
		failedAndStartedTasks := []task.Task{}

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

			// if it is inactive, roll up the version and don't create any
			// builds for it
			if !versionActive {
				if lastRolledUpVersion == nil {
					lastRolledUpVersion = &restModel.APIVersions{RolledUp: true, Versions: []restModel.APIVersion{}}
				}

				// add the version data into the last rolled-up version
				newVersion := restModel.APIVersion{}
				newVersion.BuildFromService(versionFromDB)
				lastRolledUpVersion.Versions = append(lastRolledUpVersion.Versions, newVersion)

				// move on to the next version
				continue
			}

			// add any represented build variants to the set and initialize rows
			for _, b := range buildsInVersion {
				displayName := buildVariantMappings[b.BuildVariant]
				if displayName == "" {
					displayName = b.BuildVariant
				}
				buildVariants = append(buildVariants, displayName)

				buildVariant := buildVariantMappings[b.BuildVariant]

				if buildVariant == "" {
					buildVariant = b.BuildVariant
				}

				if _, ok := buildList[b.BuildVariant]; !ok {
					buildList[b.BuildVariant] = restModel.BuildList{
						Builds:       map[string]restModel.APIBuild{},
						BuildVariant: buildVariant,
					}
				}

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
			activeVersion := restModel.APIVersion{}
			activeVersion.BuildFromService(versionFromDB)

			// add the builds to the "row"
			for _, b := range buildsInVersion {
				currentRow := buildList[b.BuildVariant]
				buildsForRow := restModel.APIBuild{}
				buildsForRow.BuildFromService(b, nil)
				buildsForRow.SetTaskCache(tasksByBuild[b.Id])

				currentRow.Builds[versionFromDB.Id] = buildsForRow
				buildList[b.BuildVariant] = currentRow
				for _, task := range tasksByBuild[b.Id] {
					if task.Status == evergreen.TaskFailed || task.Status == evergreen.TaskStarted {
						failedAndStartedTasks = append(failedAndStartedTasks, task)
					}
				}
			}

			// add the version
			finalVersions = append(finalVersions, restModel.APIVersions{RolledUp: false, Versions: []restModel.APIVersion{activeVersion}})

		}

		if err = addFailedAndStartedTests(ctx, buildList, failedAndStartedTasks); err != nil {
			return nil, err
		}
	}

	// if the last version was rolled-up, add it
	if lastRolledUpVersion != nil {
		finalVersions = append(finalVersions, *lastRolledUpVersion)
	}

	return &restModel.VersionVariantData{
		Rows:          buildList,
		Versions:      finalVersions,
		BuildVariants: buildVariants,
	}, nil

}

// addFailedAndStartedTests adds all of the failed tests associated with a task
func addFailedAndStartedTests(ctx context.Context, rows map[string]restModel.BuildList, failedAndStartedTasks []task.Task) error {
	for i := range failedAndStartedTasks {
		if err := failedAndStartedTasks[i].PopulateTestResults(ctx); err != nil {
			return errors.Wrap(err, "populating test results")
		}
	}

	failedTestsByTaskId := map[string][]string{}
	for _, t := range failedAndStartedTasks {
		failedTests := []string{}
		for _, r := range t.LocalTestResults {
			if r.Status == evergreen.TestFailedStatus {
				failedTests = append(failedTests, r.GetDisplayTestName())
			}
		}
		failedTestsByTaskId[t.Id] = failedTests
	}
	for buildVariant, row := range rows {
		for versionId, build := range row.Builds {
			for i, task := range build.TaskCache {
				if len(failedTestsByTaskId[task.Id]) != 0 {
					rows[buildVariant].Builds[versionId].TaskCache[i].FailedTestNames = append(
						rows[buildVariant].Builds[versionId].TaskCache[i].FailedTestNames,
						failedTestsByTaskId[task.Id]...)
					sort.Strings(rows[buildVariant].Builds[versionId].TaskCache[i].FailedTestNames)
				}
			}
		}
	}

	return nil
}

func (vc *DBVersionConnector) CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo,
	metadata model.VersionMetadata) (*model.Version, error) {
	newVersion, err := repotracker.CreateVersionFromConfig(ctx, projectInfo, metadata, false, nil)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "creating version from config").Error(),
		}
	}

	return newVersion, nil
}
