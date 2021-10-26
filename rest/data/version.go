package data

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/repotracker"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBVersionConnector is a struct that implements Version related methods
// from the Connector through interactions with the backing database.
type DBVersionConnector struct{}

// FindVersionById queries the backing database for the version with the given versionId.
func (vc *DBVersionConnector) FindVersionById(versionId string) (*model.Version, error) {
	v, err := model.VersionFindOne(model.VersionById(versionId))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with id %s not found", versionId),
		}
	}
	return v, nil
}

func (vc *DBVersionConnector) FindVersionByProjectAndRevision(projectId, revision string) (*model.Version, error) {
	return model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(projectId, revision))
}

func (vc *DBVersionConnector) AddGitTagToVersion(versionId string, gitTag model.GitTag) error {
	return model.AddGitTag(versionId, gitTag)
}

// AbortVersion aborts all tasks of a version given its ID.
// It wraps the service level AbortModel.Version
func (vc *DBVersionConnector) AbortVersion(versionId, caller string) error {
	return task.AbortVersion(versionId, task.AbortInfo{User: caller})
}

// RestartVersion wraps the service level RestartVersion, which restarts
// completed tasks associated with a given versionId. If abortInProgress is
// true, it also sets the abort flag on any in-progress tasks. In addition, it
// updates all builds containing the tasks affected.
func (vc *DBVersionConnector) RestartVersion(versionId string, caller string) error {
	return model.RestartTasksInVersion(versionId, true, caller)
}

func (bc *DBVersionConnector) LoadProjectForVersion(v *model.Version, projectId string) (*model.Project, *model.ParserProject, error) {
	return model.LoadProjectForVersion(v, projectId, false)
}

// GetProjectVersionsWithOptions returns the versions that fit the given constraint.
func (vc *DBVersionConnector) GetProjectVersionsWithOptions(projectName string, opts model.GetVersionsOptions) ([]restModel.APIVersion, error) {
	versions, err := model.GetVersionsWithOptions(projectName, opts)
	if err != nil {
		return nil, err
	}
	res := []restModel.APIVersion{}
	for _, v := range versions {
		apiVersion := restModel.APIVersion{}
		if err = apiVersion.BuildFromService(&v); err != nil {
			return nil, errors.Wrap(err, "error building API versions")
		}
		res = append(res, apiVersion)
	}
	return res, nil
}

// Fetch versions until 'numVersionElements' elements are created, including
// elements consisting of multiple versions rolled-up into one.
// The skip value indicates how many versions back in time should be skipped
// before starting to fetch versions, the project indicates which project the
// returned versions should be a part of.
func (vc *DBVersionConnector) GetVersionsAndVariants(skip, numVersionElements int, project *model.Project) (*restModel.VersionVariantData, error) {
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
			return nil, errors.Wrap(err,
				"error fetching versions and builds")
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
				err = newVersion.BuildFromService(&versionFromDB)
				if err != nil {
					return nil, errors.Wrapf(err, "error converting version %s from DB model", versionFromDB.Id)
				}
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
			err = activeVersion.BuildFromService(&versionFromDB)
			if err != nil {
				return nil, errors.Wrapf(err, "error converting version %s from DB model", versionFromDB.Id)
			}

			// add the builds to the "row"
			for _, b := range buildsInVersion {
				currentRow := buildList[b.BuildVariant]
				buildsForRow := restModel.APIBuild{}
				err = buildsForRow.BuildFromService(b)
				if err != nil {
					return nil, errors.Wrapf(err, "error converting build %s from DB model", b.Id)
				}
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

		if err = addFailedAndStartedTests(buildList, failedAndStartedTasks); err != nil {
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
func addFailedAndStartedTests(rows map[string]restModel.BuildList, failedAndStartedTasks []task.Task) error {
	for i := range failedAndStartedTasks {
		if err := failedAndStartedTasks[i].PopulateTestResults(); err != nil {
			return errors.Wrap(err, "populating test results")
		}
	}

	failedTestsByTaskId := map[string][]string{}
	for _, t := range failedAndStartedTasks {
		failedTests := []string{}
		for _, r := range t.LocalTestResults {
			if r.Status == evergreen.TestFailedStatus {
				failedTests = append(failedTests, r.TestFile)
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
	metadata model.VersionMetadata, active bool) (*model.Version, error) {
	newVersion, err := repotracker.CreateVersionFromConfig(ctx, projectInfo, metadata, false, nil)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("error creating version: %s", err.Error()),
		}
	}

	if active {
		catcher := grip.NewBasicCatcher()
		for _, b := range newVersion.BuildIds {
			catcher.Add(model.SetBuildActivation(b, true, evergreen.DefaultTaskActivator))
		}
		if catcher.HasErrors() {
			return nil, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("error activating builds: %s", catcher.Resolve().Error()),
			}
		}
	}

	return newVersion, nil
}

// MockVersionConnector stores a cached set of tasks that are queried against by the
// implementations of the Connector interface's Version related functions.
type MockVersionConnector struct {
	CachedTasks             []task.Task
	CachedVersions          []model.Version
	CachedRestartedVersions map[string]string
}

// FindVersionById is the mock implementation of the function for the Connector interface
// without needing to use a database. It returns results based on the cached versions in the MockVersionConnector.
func (mvc *MockVersionConnector) FindVersionById(versionId string) (*model.Version, error) {
	for _, v := range mvc.CachedVersions {
		if v.Id == versionId {
			return &v, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("build with id %s not found", versionId),
	}
}

func (mvc *MockVersionConnector) FindVersionByProjectAndRevision(projectId, revision string) (*model.Version, error) {
	for _, v := range mvc.CachedVersions {
		if v.Identifier == projectId && v.Revision == revision {
			return &v, nil
		}
	}
	return nil, nil
}

func (mvc *MockVersionConnector) AddGitTagToVersion(versionId string, gitTag model.GitTag) error {
	for _, v := range mvc.CachedVersions {
		if v.Id == versionId {
			v.GitTags = append(v.GitTags, gitTag)
		}
	}
	return errors.New("no version found")
}

// AbortVersion aborts all tasks of a version given its ID. Specifically, it sets the
// Aborted key of the tasks to true if they are currently in abortable statuses.
func (mvc *MockVersionConnector) AbortVersion(versionId, caller string) error {
	for idx, t := range mvc.CachedTasks {
		if t.Version == versionId && (t.Status == evergreen.TaskStarted || t.Status == evergreen.TaskDispatched) {
			if !t.Aborted {
				pt := &mvc.CachedTasks[idx]
				pt.Aborted = true
			}
		}
	}
	return nil
}

// The main function of the RestartVersion() for the MockVersionConnector is to
// test connectivity. It sets the value of versionId in CachedRestartedVersions
// to the caller.
func (mvc *MockVersionConnector) RestartVersion(versionId string, caller string) error {
	mvc.CachedRestartedVersions[versionId] = caller
	return nil
}

func (mvc *MockVersionConnector) GetVersionsAndVariants(skip, numVersionElements int, project *model.Project) (*restModel.VersionVariantData, error) {
	return nil, nil
}

func (mvc *MockVersionConnector) CreateVersionFromConfig(ctx context.Context, projectInfo *model.ProjectInfo, metadata model.VersionMetadata, active bool) (*model.Version, error) {
	return &model.Version{
		Requester:         evergreen.GitTagRequester,
		TriggeredByGitTag: metadata.GitTag,
	}, nil
}

func (mvc *MockVersionConnector) LoadProjectForVersion(v *model.Version, projectId string) (*model.Project, *model.ParserProject, error) {
	if v.Config != "" {
		p := &model.Project{}
		ctx := context.Background()
		pp, err := model.LoadProjectInto(ctx, []byte(v.Config), nil, projectId, p)
		return p, pp, err
	}
	return nil, nil, errors.New("no project for version")
}

func (mvc *MockVersionConnector) GetProjectVersionsWithOptions(projectId string, opts model.GetVersionsOptions) ([]restModel.APIVersion, error) {
	return nil, nil
}
