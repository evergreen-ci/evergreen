package data

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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

func (c *DBStatusConnector) FindRecentTaskListDistro(minutes int) (*task.ResultCountList, error) {
	list, err := FindRecentTaskList(minutes, task.DistroIdKey)
	return list, errors.WithStack(err)
}

func (c *DBStatusConnector) FindRecentTaskListProject(minutes int) (*task.ResultCountList, error) {
	list, err := FindRecentTaskList(minutes, task.ProjectKey)
	return list, errors.WithStack(err)
}

func FindRecentTaskList(minutes int, key string) (*task.ResultCountList, error) {
	stats, err := task.GetRecentTaskStatsList(time.Duration(minutes)*time.Minute, key)
	if err != nil {
		return nil, err
	}

	statList := task.GetResultCountList(stats)
	return &statList, nil
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
	CachedResultCountList *task.ResultCountList
	CachedHostStats       []host.StatsByDistro
}

// FindRecentTasks is a mock implementation for testing.
func (c *MockStatusConnector) FindRecentTasks(minutes int) ([]task.Task, *task.ResultCounts, error) {
	return c.CachedTasks, c.CachedResults, nil
}

func (c *MockStatusConnector) FindRecentTaskListDistro(minutes int) (*task.ResultCountList, error) {
	return c.CachedResultCountList, nil
}

func (c *MockStatusConnector) FindRecentTaskListProject(minutes int) (*task.ResultCountList, error) {
	return c.CachedResultCountList, nil
}

// GetHostStatsByDistro returns mock stats for hosts broken down by distro
func (c *MockStatusConnector) GetHostStatsByDistro() ([]host.StatsByDistro, error) {
	return c.CachedHostStats, nil
}
