package data

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/mongodb/grip"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/repotracker"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBVersionConnector is a struct that implements Version related methods
// from the Connector through interactions with the backing database.
type DBVersionConnector struct{}

// FindCostByVersionId queries the backing database for cost data associated
// with the given versionId. This is done by aggregating TimeTaken over all tasks
// of the given model.Version
func (vc *DBVersionConnector) FindCostByVersionId(versionId string) (*task.VersionCost, error) {
	pipeline := task.CostDataByVersionIdPipeline(versionId)
	res := []task.VersionCost{}

	if err := task.Aggregate(pipeline, &res); err != nil {
		return nil, err
	}

	if len(res) > 1 {
		return nil, fmt.Errorf("aggregation query with version_id %s returned %d results but should only return 1 result", versionId, len(res))
	}

	if len(res) == 0 {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with id %s not found", versionId),
		}
	}
	return &res[0], nil
}

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

// AbortVersion aborts all tasks of a version given its ID.
// It wraps the service level AbortModel.Version
func (vc *DBVersionConnector) AbortVersion(versionId, caller string) error {
	return model.AbortVersion(versionId, caller)
}

// RestartVersion wraps the service level RestartVersion, which restarts
// completed tasks associated with a given versionId. If abortInProgress is
// true, it also sets the abort flag on any in-progress tasks. In addition, it
// updates all builds containing the tasks affected.
func (vc *DBVersionConnector) RestartVersion(versionId string, caller string) error {
	// Get a list of all tasks of the given versionId
	tasks, err := task.Find(task.ByVersion(versionId))
	if err != nil {
		return err
	}
	if tasks == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with id %s not found", versionId),
		}
	}
	var taskIds []string
	for _, task := range tasks {
		taskIds = append(taskIds, task.Id)
	}
	return model.RestartVersion(versionId, taskIds, true, caller)
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
		versionsFromDB, buildsByVersion, err :=
			model.FetchVersionsAndAssociatedBuilds(project, skip, numVersionElements, true)

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

				currentRow.Builds[versionFromDB.Id] = buildsForRow
				buildList[b.BuildVariant] = currentRow
				for _, task := range buildsForRow.TaskCache {
					if task.Status == evergreen.TaskFailed || task.Status == evergreen.TaskStarted {
						failedAndStartedTaskIds = append(failedAndStartedTaskIds, task.Id)
					}
				}
			}

			// add the version
			finalVersions = append(finalVersions, restModel.APIVersions{RolledUp: false, Versions: []restModel.APIVersion{activeVersion}})

		}

		if err = addFailedAndStartedTests(buildList, failedAndStartedTaskIds); err != nil {
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

// addFailedAndStartedTests adds all of the failed tests associated with a task
func addFailedAndStartedTests(rows map[string]restModel.BuildList, failedAndStartedTaskIds []string) error {
	failedAndStartedTasks, err := task.Find(task.ByIds(failedAndStartedTaskIds))
	if err != nil {
		return errors.Wrap(err, "error fetching failed tasks")

	}

	for i := range failedAndStartedTasks {
		if err := failedAndStartedTasks[i].MergeNewTestResults(); err != nil {
			return errors.Wrap(err, "error merging test results")
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

func (vc *DBVersionConnector) CreateVersionFromConfig(ctx context.Context, projectID string, config []byte, user *user.DBUser, message string, active bool) (*model.Version, error) {
	ref, err := model.FindOneProjectRef(projectID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "error finding project",
		}
	}
	if ref == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project %s does not exist", projectID),
		}
	}
	project := &model.Project{}
	err = model.LoadProjectInto(config, projectID, project)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("error parsing project config: %s", err.Error()),
		}
	}
	metadata := repotracker.VersionMetadata{
		IsAdHoc: true,
		User:    user,
		Message: message,
	}
	newVersion, err := repotracker.CreateVersionFromConfig(ctx, ref, project, metadata, false, nil)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("error creating version: %s", err.Error()),
		}
	}

	if active {
		catcher := grip.NewBasicCatcher()
		for _, b := range newVersion.BuildIds {
			catcher.Add(model.SetBuildActivation(b, true, evergreen.DefaultTaskActivator, true))
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

// FindCostByVersionId is the mock implementation of the function for the Connector interface
// without needing to use a database. It returns results based on the cached tasks in the MockVersionConnector.
func (mvc *MockVersionConnector) FindCostByVersionId(versionId string) (*task.VersionCost, error) {
	vc := task.VersionCost{
		VersionId:    "",
		SumTimeTaken: 0,
	}

	// Simulate aggregation
	for _, t := range mvc.CachedTasks {
		if t.Version == versionId {
			if vc.VersionId == "" {
				vc.VersionId = versionId
			}
			vc.SumTimeTaken += t.TimeTaken
		}
	}

	// Throw an error when no task with the given version id is found
	if vc.VersionId == "" {
		return nil, fmt.Errorf("no task with version_id %s has been found", versionId)
	}
	return &vc, nil
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

func (mvc *MockVersionConnector) CreateVersionFromConfig(ctx context.Context, projectID string, config []byte, user *user.DBUser, message string, active bool) (*model.Version, error) {
	return nil, nil
}
