package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBDistroConnector is a struct that implements the Distro related methods
// from the Connector through interactions with the backing database.
type DBDistroConnector struct{}

// FindDistroById queries the database to find a given distros.
func (dc *DBDistroConnector) FindDistroById(distroId string) (*distro.Distro, error) {
	d, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("distro with id '%s' not found", distroId),
		}
	}
	return &d, nil
}

// FindAllDistros queries the database to find all distros.
func (dc *DBDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	distros, err := distro.Find(distro.All)
	if err != nil {
		return nil, err
	}
	if distros == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("no distros found"),
		}
	}
	return distros, nil
}

// UpdateDistro updates the given distro.Distro.
func (dc *DBDistroConnector) UpdateDistro(old, new *distro.Distro) error {
	if old.Id != new.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("programmer error updating distro %s [%s]", new.Id, old.Id),
		}

	}

	if old.DispatcherSettings.Version == evergreen.DispatcherVersionRevisedWithDependencies && new.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
		if err := model.RemoveTaskQueues(new.Id); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("could not clear invalid task queues for %s", new.Id),
			}
		}
	}

	err := new.Update()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not updated", new.Id),
		}
	}
	return nil
}

// CreateDistro inserts the given distro.Distro.
func (dc *DBDistroConnector) CreateDistro(distro *distro.Distro) error {
	err := distro.Insert()
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not inserted", distro.Id),
		}
	}
	return nil
}

// DeleteDistroById removes a given distro from the database based on its id.
func (dc *DBDistroConnector) DeleteDistroById(distroId string) error {
	var err error
	if err = host.MarkInactiveStaticHosts([]string{}, distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("hosts for distro with id '%s' were not terminated", distroId),
		}
	}
	if err = distro.Remove(distroId); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("distro with id '%s' was not deleted", distroId),
		}
	}
	return nil
}

// FindCostByDistroId queries the backing database for cost data associated
// with the given distroId. This is done by aggregating TimeTaken over all
// tasks of the given distro that match the time range.
func (tc *DBDistroConnector) FindCostByDistroId(distroId string,
	starttime time.Time, duration time.Duration) (*task.DistroCost, error) {
	// First look for the distro with the given ID.
	// If the find fails, return an error.
	d, err := distro.FindOne(distro.ById(distroId))
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
	dc.Provider = d.Provider
	dc.ProviderSettings = *(d.ProviderSettings)

	return &dc, nil
}

// ClearTaskQueue deletes all tasks from the task queue for a distro
func (tc *DBDistroConnector) ClearTaskQueue(distroId string) error {
	return model.ClearTaskQueue(distroId)
}

// MockDistroConnector is a struct that implements mock versions of
// Distro-related methods for testing.
type MockDistroConnector struct {
	CachedDistros []*distro.Distro
	CachedTasks   []task.Task
}

func (mdc *MockDistroConnector) FindDistroById(distroId string) (*distro.Distro, error) {
	// Find the distro.
	for _, d := range mdc.CachedDistros {
		if d.Id == distroId {
			return d, nil
		}
	}
	return nil, gimlet.ErrorResponse{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("distro with id '%s' not found", distroId),
	}
}

// FindAllDistros is a mock implementation for testing.
func (mdc *MockDistroConnector) FindAllDistros() ([]distro.Distro, error) {
	out := []distro.Distro{}
	for _, d := range mdc.CachedDistros {
		out = append(out, *d)
	}
	return out, nil
}

func (mdc *MockDistroConnector) UpdateDistro(old, new *distro.Distro) error {
	for _, d := range mdc.CachedDistros {
		if d.Id == new.Id {
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("distro with id '%s' was not updated", new.Id),
	}
}

func (mdc *MockDistroConnector) DeleteDistroById(distroId string) error {
	for _, d := range mdc.CachedDistros {
		if d.Id == distroId {
			return nil
		}
	}
	return gimlet.ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		Message:    fmt.Sprintf("distro with id '%s' was not deleted", distroId),
	}
}

func (mdc *MockDistroConnector) CreateDistro(distro *distro.Distro) error {
	for _, d := range mdc.CachedDistros {
		if d.Id == distro.Id {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("distro with id '%s' was not inserted", distro.Id),
			}
		}
	}
	return nil
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

func (mdc *MockDistroConnector) ClearTaskQueue(distroId string) error {
	return errors.New("ClearTaskQueue unimplemented for mock")
}
