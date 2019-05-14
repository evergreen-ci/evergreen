package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// DBStatusConnector is a struct that implements the status related methods
// from the Connector through interactions with the backing database.
type DBStatusConnector struct{}

// FindRecentTasks queries the database to find all distros.
func (c *DBStatusConnector) FindRecentTasks(minutes int) ([]task.Task, *task.ResultCounts, error) {
	tasks, err := task.GetRecentTasks(time.Duration(minutes) * time.Minute)
	if err != nil {
		return nil, nil, err
	}

	if tasks == nil {
		return []task.Task{}, &task.ResultCounts{}, err
	}
	stats := task.GetResultCounts(tasks)
	return tasks, stats, nil
}

func (c *DBStatusConnector) FindRecentTaskListDistro(minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(minutes, task.DistroIdKey)
	return apiList, errors.WithStack(err)
}

func (c *DBStatusConnector) FindRecentTaskListProject(minutes int) (*model.APIRecentTaskStatsList, error) {
	apiList, err := FindRecentTaskList(minutes, task.ProjectKey)
	return apiList, errors.WithStack(err)
}

func FindRecentTaskList(minutes int, key string) (*model.APIRecentTaskStatsList, error) {
	stats, err := task.GetRecentTaskStats(time.Duration(minutes)*time.Minute, key)
	if err != nil {
		return nil, errors.Wrap(err, "can't get task stats")
	}
	list := task.GetResultCountList(stats)

	apiList := model.APIRecentTaskStatsList{}
	if err := apiList.BuildFromService(list); err != nil {
		return nil, errors.Wrap(err, "can't convert to API")
	}

	return &apiList, nil
}

// GetHostStatsByDistro returns counts of up hosts broken down by distro
func (c *DBStatusConnector) GetHostStatsByDistro() ([]host.StatsByDistro, error) {
	return host.GetStatsByDistro()
}

// MockStatusConnector is a struct that implements mock versions of
// Distro-related methods for testing.
type MockStatusConnector struct {
	CachedTasks           []task.Task
	CachedResults         *task.ResultCounts
	CachedResultCountList *model.APIRecentTaskStatsList
	CachedHostStats       []host.StatsByDistro
}

// FindRecentTasks is a mock implementation for testing.
func (c *MockStatusConnector) FindRecentTasks(minutes int) ([]task.Task, *task.ResultCounts, error) {
	return c.CachedTasks, c.CachedResults, nil
}

func (c *MockStatusConnector) FindRecentTaskListDistro(minutes int) (*model.APIRecentTaskStatsList, error) {
	return c.CachedResultCountList, nil
}

func (c *MockStatusConnector) FindRecentTaskListProject(minutes int) (*model.APIRecentTaskStatsList, error) {
	return c.CachedResultCountList, nil
}

// GetHostStatsByDistro returns mock stats for hosts broken down by distro
func (c *MockStatusConnector) GetHostStatsByDistro() ([]host.StatsByDistro, error) {
	return c.CachedHostStats, nil
}
