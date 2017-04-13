package servicecontext

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// DBTaskConnector is a struct that implements the Task related methods
// from the ServiceContext through interactions with he backing database.
type DBTaskConnector struct{}

// FindTaskById uses the service layer's task type to query the backing database for
// the task with the given taskId.
func (tc *DBTaskConnector) FindTaskById(taskId string) (*task.Task, error) {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, &apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task with id %s not found", taskId),
		}
	}
	return t, nil
}

// FindTasksByBuildId uses the service layer's task type to query the backing database for a
// list of task that matches buildId. It accepts the startTaskId and a limit
// to allow for pagination of the queries. It returns results sorted by taskId.
func (tc *DBTaskConnector) FindTasksByBuildId(buildId, startTaskId string, limit int) ([]task.Task, error) {
	var ts []task.Task
	var err error
	// If we have specified a taskId to start the iteration from, then search
	// for it and fetch the list of tasks starting there.
	if startTaskId != "" {
		ts, err = task.Find(task.ByBuildIdAfterTaskId(buildId, startTaskId).Limit(limit))
		if err != nil {
			return nil, err
		}
		// Otherwise, begin the iteration from the beginning of the list of tasks.
	} else {
		ts, err = task.Find(task.ByBuildId(buildId).Sort([]string{"+" + task.IdKey}).Limit(limit))
		if err != nil {
			return nil, err
		}
	}
	if len(ts) == 0 {
		return nil, &apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("tasks with buildId %v not found", buildId),
		}
	}
	return ts, nil
}

func (tc *DBTaskConnector) FindTasksByIds(ids []string) ([]task.Task, error) {
	ts, err := task.Find(task.ByIds(ids))
	if err != nil {
		return nil, err
	}
	if len(ts) == 0 {
		return []task.Task{}, &apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    "no tasks found",
		}
	}
	return ts, nil
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (tc *DBTaskConnector) SetTaskPriority(t *task.Task, priority int64) error {
	err := t.SetPriority(priority)
	return err
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (tc *DBTaskConnector) SetTaskActivated(taskId, user string, activated bool) error {
	return errors.Wrap(serviceModel.SetActiveState(taskId, user, activated),
		"Erorr setting task active")
}

// ResetTask sets the task to be in an unexecuted state and prepares it to be
// run again.
func (tc *DBTaskConnector) ResetTask(taskId, username string, proj *serviceModel.Project) error {
	return errors.Wrap(serviceModel.TryResetTask(taskId, username, evergreen.RESTV2Package, proj, nil),
		"Reset task error")
}

// MockTaskConnector stores a cached set of tasks that are queried against by the
// implementations of the ServiceContext interface's Task related functions.
type MockTaskConnector struct {
	CachedTasks []task.Task
	StoredError error
}

// FindTaskById provides a mock implementation of the functions for the
// ServiceContext interface without needing to use a database. It returns results
// based on the cached tasks in the MockTaskConnector.
func (mdf *MockTaskConnector) FindTaskById(taskId string) (*task.Task, error) {
	for _, t := range mdf.CachedTasks {
		if t.Id == taskId {
			return &t, mdf.StoredError
		}
	}
	return nil, mdf.StoredError
}

func (mdf *MockTaskConnector) FindTasksByIds(taskIds []string) ([]task.Task, error) {
	return mdf.CachedTasks, mdf.StoredError
}

// FindTaskByBuildId provides a mock implementation of the function for the
// ServiceContext interface without needing to use a database. It returns results
// based on the cached tasks in the MockTaskConnector.
func (mdf *MockTaskConnector) FindTasksByBuildId(buildId, startTaskId string, limit int) ([]task.Task, error) {
	return mdf.CachedTasks, mdf.StoredError
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (mdf *MockTaskConnector) SetTaskPriority(it *task.Task, priority int64) error {
	for ix, t := range mdf.CachedTasks {
		if t.Id == it.Id {
			mdf.CachedTasks[ix].Priority = priority
			return mdf.StoredError
		}
	}
	return mdf.StoredError
}

// SetTaskActivated changes the activation value of a task using a call to the
// service layer function.
func (mdf *MockTaskConnector) SetTaskActivated(taskId, user string, activated bool) error {
	for ix, t := range mdf.CachedTasks {
		if t.Id == taskId {
			mdf.CachedTasks[ix].Activated = activated
			mdf.CachedTasks[ix].ActivatedBy = user
			return mdf.StoredError
		}
	}
	return mdf.StoredError
}

func (mdf *MockTaskConnector) ResetTask(taskId, username string, proj *serviceModel.Project) error {
	for ix, t := range mdf.CachedTasks {
		if t.Id == taskId {
			t.Activated = true
			t.Secret = "new secret"
			t.Status = evergreen.TaskUndispatched
			t.DispatchTime = util.ZeroTime
			t.StartTime = util.ZeroTime
			t.ScheduledTime = util.ZeroTime
			t.FinishTime = util.ZeroTime
			t.TestResults = []task.TestResult{}
			mdf.CachedTasks[ix] = t
		}
	}
	return mdf.StoredError
}
