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

// FindCostByDistroId queries the backing database for cost data associated
// with the given distroId. This is done by aggregating TimeTaken over all tasks
// of the given distro.
func (tc *DBDistroConnector) FindCostByDistroId(distroId string) (*task.DistroCost, error) {
	// Run aggregation with the aggregation pipeline
	pipeline := task.CostDataByDistroIdPipeline(distroId)
	res := []task.DistroCost{}
	if err := task.Aggregate(pipeline, &res); err != nil {
		return nil, err
	}

	// Account for possible error cases
	if len(res) > 1 {
		return nil, errors.Errorf("aggregation query with distro_id %s returned %d results but should only return 1 result", distroId, len(res))
	}

	if len(res) == 0 {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro with id %s not found", distroId),
		}
	}

	// Add additional information by finding the distro with the given Id.
	// In particular, provider and provider settings are added to the
	// DistroCost model.
	dc := res[0]
	distro, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, err
	}
	dc.Provider = distro.Provider
	dc.ProviderSettings = *(distro.ProviderSettings)

	return &dc, nil
}

// MockDistroConnector is a struct that implements mock versions of
// Distro-related methods for testing.
type MockDistroConnector struct {
	CachedDistros []distro.Distro
	CachedTasks   []task.Task
}

// FindAllDistros is a mock implementation for testing.
func (mdc *MockDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	return mdc.CachedDistros, nil
}

// FindCostByDistroId returns results based on the cached tasks and cached distros
// in the MockDistroConnector.
func (mdc *MockDistroConnector) FindCostByDistroId(distroId string) (*task.DistroCost, error) {
	dc := task.DistroCost{}

	// Simulate aggregation
	for _, t := range mdc.CachedTasks {
		if t.DistroId == distroId {
			if dc.DistroId == "" {
				dc.DistroId = distroId
			}
			dc.SumTimeTaken += t.TimeTaken
		}
	}

	// Find the distro and add the provider information to DistroCost.
	for _, d := range mdc.CachedDistros {
		if d.Id == distroId {
			dc.Provider = d.Provider
			dc.ProviderSettings = *(d.ProviderSettings)
		}
	}

	// Throw an error when no task with the given distro ID is found.
	if dc.DistroId == "" {
		return nil, fmt.Errorf("no task with distro_id %s has been found", distroId)
	}

	// Throw an error when there is no provider information given with the distro
	if dc.Provider == "" {
		return nil, fmt.Errorf("no provider has been found with distro %s", distroId)
	}
	return &dc, nil
}
