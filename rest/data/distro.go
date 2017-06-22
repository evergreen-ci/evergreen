package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/pkg/errors"
)

// DBDistroConnector is a struct that implements the Distro related methods
// from the Connector through interactions with the backing database.
type DBDistroConnector struct{}

// FindAllDistros queries the database to find all distros.
func (dc *DBDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, err
	}
	if distros == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no distros found"),
		}
	}
	return distros, nil
}

// MockDistroConnector is a struct that implements Distro-related methods
// for testing.
type MockDistroConnector struct {
	Distros []distro.Distro
}

// FindAllDistros is a mock implementation for testing.
func (dc *MockDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	return dc.Distros, nil
}

// FindCostByDistroId queries the backing database for cost data associated
// with the given distroId. This is done by aggregating TimeTaken over all tasks
// of the given distro.
func (tc *DBDistroConnector) FindCostByDistroId(distroId string) (*task.DistroCost, error) {
	pipeline := task.CostDataByDistroIdPipeline(distroId)
	res := []task.DistroCost{}

	if err := task.Aggregate(pipeline, &res); err != nil {
		return nil, err
	}

	if len(res) > 1 {
		return nil, errors.Errorf("aggregation query with distro_id %s returned %d results but should only return 1 result", distroId, len(res))
	}

	if len(res) == 0 {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro with id %s not found", distroId),
		}
	}
	return &res[0], nil
}

// MockDistroCostConnector stores a cached set of tasks that are queried against by the
// implementations of the Connector interface's Distro Cost related functions.
type MockDistroCostConnector struct {
	CachedTasks []task.Task
}

// FindCostByDistroId is the mock implementation of the function for the Connector interface
// without needing to use a database. It returns results based on the cached tasks in the MockDistroCostConnector.
func (mdcc *MockDistroCostConnector) FindCostByDistroId(distroId string) (*task.DistroCost, error) {
	dc := task.DistroCost{
		DistroId:     "",
		SumTimeTaken: 0,
	}

	// Simulate aggregation
	for _, t := range mdcc.CachedTasks {
		if t.DistroId == distroId {
			if dc.DistroId == "" {
				dc.DistroId = distroId
			}
			dc.SumTimeTaken += t.TimeTaken
		}
	}

	// Throw an error when no task with the given distro id is found
	if dc.DistroId == "" {
		return nil, fmt.Errorf("no task with distro_id %s has been found", distroId)
	}
	return &dc, nil
}
