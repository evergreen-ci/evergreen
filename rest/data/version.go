package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBVersionConnector is a struct that implements Version related methods
// from the Connector through interactions with the backing database.
type DBVersionConnector struct{}

// FindCostByVersionId queries the backing database for cost data associated
// with the given versionId. This is done by aggregating TimeTaken over all tasks
// of the given version.
func (tc *DBVersionConnector) FindCostByVersionId(versionId string) (*task.VersionCost, error) {
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

// MockVersionConnector stores a cached set of tasks that are queried against by the
// implementations of the Connector interface's Version related functions.
type MockVersionConnector struct {
	CachedTasks []task.Task
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
