package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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

// GetHostStatsByDistro returns counts of up hosts broken down by distro
func (c *DBStatusConnector) GetHostStatsByDistro() ([]host.HostStatsByDistro, error) {
	return host.GetHostStatsByDistro()
}

// MockStatusConnector is a struct that implements mock versions of
// Distro-related methods for testing.
type MockStatusConnector struct {
	CachedTasks     []task.Task
	CachedResults   *task.ResultCounts
	CachedHostStats []host.HostStatsByDistro
}

// FindRecentTasks is a mock implementation for testing.
func (c *MockStatusConnector) FindRecentTasks(minutes int) ([]task.Task, *task.ResultCounts, error) {
	return c.CachedTasks, c.CachedResults, nil
}

// GetHostStatsByDistro returns mock stats for hosts broken down by distro
func (c *MockStatusConnector) GetHostStatsByDistro() ([]host.HostStatsByDistro, error) {
	return c.CachedHostStats, nil
}
