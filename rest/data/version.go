package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBVersionConnector is a struct that implements Version related methods
// from the Connector through interactions with the backing database.
type DBVersionConnector struct{}

// FindCostByVersionId queries the backing database for cost data associated
// with the given versionId. This is done by aggregating TimeTaken over all tasks
// of the given version.
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
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with id %s not found", versionId),
		}
	}
	return &res[0], nil
}

// FindVersionById queries the backing database for the version with the given versionId.
func (vc *DBVersionConnector) FindVersionById(versionId string) (*version.Version, error) {
	v, err := version.FindOne(version.ById(versionId))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with id %s not found", versionId),
		}
	}
	return v, nil
}

func (vc *DBVersionConnector) FindActivatedVersionsByProjectId(projectId string) ([]version.Version, error) {
	versions, err := version.Find(version.ByProjectIdActivated(projectId))

	if err != nil {
		return nil, err
	}

	if versions == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("No versions found for %s project", projectId),
		}
	}
	return versions, nil
}

// AbortVersion aborts all tasks of a version given its ID.
// It wraps the service level AbortVersion.
func (vc *DBVersionConnector) AbortVersion(versionId string) error {
	return model.AbortVersion(versionId)
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
		return &rest.APIError{
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

// MockVersionConnector stores a cached set of tasks that are queried against by the
// implementations of the Connector interface's Version related functions.
type MockVersionConnector struct {
	CachedTasks             []task.Task
	CachedVersions          []version.Version
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
func (mvc *MockVersionConnector) FindVersionById(versionId string) (*version.Version, error) {
	for _, v := range mvc.CachedVersions {
		if v.Id == versionId {
			return &v, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("build with id %s not found", versionId),
	}
}

// FindActivatedVersionsByProjectId is the mock implementation of the function for the Connector interface
// without needing to use a database. It returns results based on the cached versions in the MockVersionConnector.
func (mvc *MockVersionConnector) FindActivatedVersionsByProjectId(projectId string) ([]version.Version, error) {
	for _, v := range mvc.CachedVersions {
		isActivated := false
		for _, bv := range v.BuildVariants {
			if bv.Activated {
				isActivated = true
				break
			}
		}
		if isActivated == true && v.Identifier == projectId {
			return []version.Version{v}, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("version for project id %s not found", projectId),
	}
}

// AbortVersion aborts all tasks of a version given its ID. Specifically, it sets the
// Aborted key of the tasks to true if they are currently in abortable statuses.
func (mvc *MockVersionConnector) AbortVersion(versionId string) error {
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
