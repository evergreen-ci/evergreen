package data

import (
	"fmt"
	"net/http"
	"time"

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
// with the given distroId. This is done by aggregating TimeTaken over all
// tasks of the given distro that match the time range.
func (tc *DBDistroConnector) FindCostByDistroId(distroId string,
	starttime time.Time, duration time.Duration) (*task.DistroCost, error) {
	// First look for the distro with the given ID.
	// If the find fails, return an error.
	distro, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, errors.Wrapf(err, "error finding distro with id %s", distroId)
	}

	// Run aggregation with the aggregation pipeline
	pipeline := task.CostDataByDistroIdPipeline(distroId, starttime, duration)
	res := []task.DistroCost{}
	if err := task.Aggregate(pipeline, &res); err != nil {
		return nil, err
	}

	// Account for possible error cases
	if len(res) > 1 {
		return nil, errors.Errorf("aggregation query with distro_id %s returned %d results but should only return 1 result", distroId, len(res))
	}
	// Aggregation ran but no tasks of given time range was found for this distro.
	if len(res) == 0 {
		return &task.DistroCost{DistroId: distroId}, nil
	}

	// Add provider and provider settings of the distro to the
	// DistroCost model.
	dc := res[0]
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

// FindCostByDistroId returns results based on the cached tasks and
// cached distros in the MockDistroConnector.
func (mdc *MockDistroConnector) FindCostByDistroId(distroId string,
	starttime time.Time, duration time.Duration) (*task.DistroCost, error) {
	dc := task.DistroCost{}
	var provider string
	var settings map[string]interface{}

	// Find the distro.
	for _, d := range mdc.CachedDistros {
		if d.Id == distroId {
			dc.DistroId = distroId
			provider = d.Provider
			settings = *d.ProviderSettings
		}
	}

	// Throw an error when no task with the given distro ID is found.
	if dc.DistroId == "" {
		return nil, fmt.Errorf("no task with distro_id %s has been found", distroId)
	}

	// Simulate aggregation
	for _, t := range mdc.CachedTasks {
		if t.DistroId == distroId && (t.FinishTime.Sub(starttime) >= 0) &&
			(starttime.Add(duration).Sub(t.FinishTime) >= 0) {
			dc.SumTimeTaken += t.TimeTaken
		}
	}

	if dc.SumTimeTaken != time.Duration(0) {
		dc.Provider = provider
		dc.ProviderSettings = settings
	}

	return &dc, nil
}
