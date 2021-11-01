package data

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// DBTaskConnector is a struct that implements the Task related methods
// from the Connector through interactions with he backing database.
type DBTaskConnector struct{}

// FindTaskById uses the service layer's task type to query the backing database for
// the task with the given taskId.
func (tc *DBTaskConnector) FindTaskById(taskId string) (*task.Task, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task with id %s not found", taskId),
		}
	}
	return t, nil
}

func (tc *DBTaskConnector) FindTaskByIdAndExecution(taskId string, execution int) (*task.Task, error) {
	t, err := task.FindOneIdAndExecution(taskId, execution)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task with id '%s' not found", taskId),
		}
	}
	return t, nil
}

func (tc *DBTaskConnector) FindTaskWithinTimePeriod(startedAfter, finishedBefore time.Time,
	project string, statuses []string) ([]task.Task, error) {
	id, err := model.GetIdForProject(project)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"func":    "FindTaskWithinTimePeriod",
			"message": "error getting id for project",
			"project": project,
		}))
		// don't return an error here to preserve existing behavior
		return nil, nil
	}

	tasks, err := task.Find(task.WithinTimePeriod(startedAfter, finishedBefore, id, statuses))

	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// FindTasksByBuildId uses the service layer's task type to query the backing database for a
// list of task that matches buildId. It accepts the startTaskId and a limit
// to allow for pagination of the queries. It returns results sorted by taskId.
func (tc *DBTaskConnector) FindTasksByBuildId(buildId, taskId, status string, limit int, sortDir int) ([]task.Task, error) {
	pipeline := task.TasksByBuildIdPipeline(buildId, taskId, status, limit, sortDir)
	res := []task.Task{}

	err := task.Aggregate(pipeline, &res)
	if err != nil {
		return []task.Task{}, err
	}

	if taskId != "" {
		found := false
		for _, t := range res {
			if t.Id == taskId {
				found = true
				break
			}
		}
		if !found {
			return []task.Task{}, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task with id '%s' not found", taskId),
			}
		}
	}
	return res, nil
}

func (tc *DBTaskConnector) FindTasksByIds(ids []string) ([]task.Task, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ts, err := task.Find(task.ByIds(ids))
	if err != nil {
		return nil, err
	}
	if len(ts) == 0 {
		return []task.Task{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no tasks found",
		}
	}
	return ts, nil
}

func (tc *DBTaskConnector) FindOldTasksByIDWithDisplayTasks(id string) ([]task.Task, error) {
	ts, err := task.FindOldWithDisplayTasks(task.ByOldTaskID(id))
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func (tc *DBTaskConnector) FindTasksByProjectAndCommit(project, commitHash, taskId, status string, limit int) ([]task.Task, error) {
	projectId, err := model.GetIdForProject(project)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		}
	}

	pipeline := task.TasksByProjectAndCommitPipeline(projectId, commitHash, taskId, status, limit)

	res := []task.Task{}
	err = task.Aggregate(pipeline, &res)
	if err != nil {
		return []task.Task{}, err
	}
	if len(res) == 0 {
		var message string
		if status != "" {
			message = fmt.Sprintf("task from project '%s' and commit '%s' with status '%s' "+
				"not found", projectId, commitHash, status)
		} else {
			message = fmt.Sprintf("task from project '%s' and commit '%s' not found",
				projectId, commitHash)
		}
		return []task.Task{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    message,
		}
	}

	if taskId != "" {
		found := false
		for _, t := range res {
			if t.Id == taskId {
				found = true
				break
			}
		}
		if !found {
			return []task.Task{}, gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task with id '%s' not found", taskId),
			}
		}
	}
	return res, nil
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (tc *DBTaskConnector) SetTaskPriority(t *task.Task, user string, priority int64) error {
	if t == nil {
		return errors.New("task cannot be nil")
	}
	return model.SetTaskPriority(*t, priority, user)
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (tc *DBTaskConnector) SetTaskActivated(taskId, user string, activated bool) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.Wrapf(err, "problem finding task '%s'", t)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", t.Id)
	}

	return errors.Wrap(serviceModel.SetActiveState(t, user, activated),
		"Erorr setting task active")
}

// ResetTask sets the task to be in an unexecuted state and prepares it to be
// run again.
func (tc *DBTaskConnector) ResetTask(taskId, username string) error {
	return errors.Wrap(serviceModel.TryResetTask(taskId, username, evergreen.RESTV2Package, nil),
		"Reset task error")
}

func (tc *DBTaskConnector) AbortTask(taskId string, user string) error {
	return serviceModel.AbortTask(taskId, user)
}

func (tc *DBTaskConnector) CheckTaskSecret(taskID string, r *http.Request) (int, error) {
	_, code, err := serviceModel.ValidateTask(taskID, true, r)
	if code == http.StatusConflict {
		return http.StatusUnauthorized, errors.New("Not authorized")
	}
	return code, errors.WithStack(err)
}

// GetManifestByTask finds the manifest corresponding to the given task.
func (tc *DBTaskConnector) GetManifestByTask(taskId string) (*manifest.Manifest, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding task '%s'", t)
	}
	if t == nil {
		return nil, errors.Errorf("task '%s' not found", t.Id)
	}
	mfest, err := manifest.FindFromVersion(t.Version, t.Project, t.Revision, t.Requester)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding manifest from version '%s'", t.Version)
	}
	if mfest == nil {
		return nil, errors.Errorf("no manifest found for version '%s'", t.Version)
	}
	return mfest, nil
}

type TaskFilterOptions struct {
	Statuses              []string
	BaseStatuses          []string
	Variants              []string
	TaskNames             []string
	Page                  int
	Limit                 int
	FieldsToProject       []string
	Sorts                 []task.TasksSortOrder
	IncludeExecutionTasks bool
	IncludeBaseTasks      bool
}

// FindTasksByVersion gets all tasks for a specific version
// Results can be filtered by task name, variant name and status in addition to being paginated and limited
func (tc *DBTaskConnector) FindTasksByVersion(versionID string, opts TaskFilterOptions) ([]task.Task, int, error) {
	getTaskByVersionOpts := task.GetTasksByVersionOptions{
		Statuses:              opts.Statuses,
		BaseStatuses:          opts.BaseStatuses,
		Variants:              opts.Variants,
		TaskNames:             opts.TaskNames,
		Page:                  opts.Page,
		Limit:                 opts.Limit,
		FieldsToProject:       opts.FieldsToProject,
		Sorts:                 opts.Sorts,
		IncludeExecutionTasks: opts.IncludeExecutionTasks,
		IncludeBaseTasks:      opts.IncludeBaseTasks,
	}
	tasks, total, err := task.GetTasksByVersion(versionID, getTaskByVersionOpts)
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// MockTaskConnector stores a cached set of tasks that are queried against by the
// implementations of the Connector interface's Task related functions.
type MockTaskConnector struct {
	CachedTasks    []task.Task
	CachedOldTasks []task.Task
	Manifests      []manifest.Manifest
	CachedAborted  map[string]string
	StoredError    error
	FailOnAbort    bool
}

// FindTaskById provides a mock implementation of the functions for the
// Connector interface without needing to use a database. It returns results
// based on the cached tasks in the MockTaskConnector.
func (mtc *MockTaskConnector) FindTaskById(taskId string) (*task.Task, error) {
	for _, t := range mtc.CachedTasks {
		if t.Id == taskId {
			return &t, mtc.StoredError
		}
	}
	return nil, mtc.StoredError
}

func (mtc *MockTaskConnector) FindTaskByIdAndExecution(taskId string, execution int) (*task.Task, error) {
	for _, t := range mtc.CachedTasks {
		if t.Id == taskId && t.Execution == execution {
			return &t, mtc.StoredError
		}
	}
	return nil, mtc.StoredError
}

func (mtc *MockTaskConnector) FindTasksByProjectAndCommit(projectId, commitHash, taskId, status string, limit int) ([]task.Task, error) {
	if mtc.StoredError != nil {
		return []task.Task{}, mtc.StoredError
	}

	ofProjectAndCommit := []task.Task{}
	for _, t := range mtc.CachedTasks {
		if t.Project == projectId && t.Revision == commitHash {
			ofProjectAndCommit = append(ofProjectAndCommit, t)
		}
	}

	// loop until the filename is found
	for ix, t := range ofProjectAndCommit {
		if t.Id == taskId {
			// We've found the task
			var tasksToReturn []task.Task
			if ix+limit > len(ofProjectAndCommit) {
				tasksToReturn = ofProjectAndCommit[ix:]
			} else {
				tasksToReturn = ofProjectAndCommit[ix : ix+limit]
			}
			return tasksToReturn, nil
		}
	}
	return nil, nil
}

func (mtc *MockTaskConnector) FindTaskWithinTimePeriod(startedAfter, finishedBefore time.Time, project string, status []string) ([]task.Task, error) {
	return mtc.CachedTasks, mtc.StoredError
}

func (mtc *MockTaskConnector) FindOldTasksByIDWithDisplayTasks(id string) ([]task.Task, error) {
	return mtc.CachedOldTasks, mtc.StoredError
}

func (mtc *MockTaskConnector) FindTasksByIds(taskIds []string) ([]task.Task, error) {
	return mtc.CachedTasks, mtc.StoredError
}

// FindTaskByBuildId provides a mock implementation of the function for the
// Connector interface without needing to use a database. It returns results
// based on the cached tasks in the MockTaskConnector.
func (mtc *MockTaskConnector) FindTasksByBuildId(buildId, startTaskId, status string, limit, sortDir int) ([]task.Task, error) {
	if mtc.StoredError != nil {
		return []task.Task{}, mtc.StoredError
	}
	ofBuildIdAndStatus := []task.Task{}
	for _, t := range mtc.CachedTasks {
		if t.BuildId == buildId {
			if status == "" || t.Status == status {
				ofBuildIdAndStatus = append(ofBuildIdAndStatus, t)
			}
		}
	}

	// loop until the start task is found
	for ix, t := range ofBuildIdAndStatus {
		if t.Id == startTaskId {
			// We've found the task
			var tasksToReturn []task.Task
			if sortDir < 0 {
				if ix-limit > 0 {
					tasksToReturn = mtc.CachedTasks[ix-(limit) : ix]
				} else {
					tasksToReturn = mtc.CachedTasks[:ix]
				}
			} else {
				if ix+limit > len(mtc.CachedTasks) {
					tasksToReturn = mtc.CachedTasks[ix:]
				} else {
					tasksToReturn = mtc.CachedTasks[ix : ix+limit]
				}
			}
			return tasksToReturn, nil
		}
	}
	return nil, nil
}

// SetTaskPriority changes the priority value of a task using a call to the
// service layer function.
func (mtc *MockTaskConnector) SetTaskPriority(it *task.Task, user string, priority int64) error {
	for ix, t := range mtc.CachedTasks {
		if t.Id == it.Id {
			mtc.CachedTasks[ix].Priority = priority
			return mtc.StoredError
		}
	}
	return mtc.StoredError
}

// SetTaskActivated changes the activation value of a task using a call to the
// service layer function.
func (mtc *MockTaskConnector) SetTaskActivated(taskId, user string, activated bool) error {
	for ix, t := range mtc.CachedTasks {
		if t.Id == taskId {
			mtc.CachedTasks[ix].Activated = activated
			mtc.CachedTasks[ix].ActivatedBy = user
			return mtc.StoredError
		}
	}
	return mtc.StoredError
}

func (mtc *MockTaskConnector) ResetTask(taskId, username string) error {
	for ix, t := range mtc.CachedTasks {
		if t.Id == taskId {
			t.Activated = true
			t.Secret = "new secret"
			t.Status = evergreen.TaskUndispatched
			t.DispatchTime = utility.ZeroTime
			t.StartTime = utility.ZeroTime
			t.ScheduledTime = utility.ZeroTime
			t.FinishTime = utility.ZeroTime
			t.LocalTestResults = []task.TestResult{}
			mtc.CachedTasks[ix] = t
		}
	}
	return mtc.StoredError
}

func (tc *MockTaskConnector) AbortTask(taskId, user string) error {
	if tc.FailOnAbort {
		return errors.New("manufactured fail")
	}
	tc.CachedAborted[taskId] = user
	return nil
}

func (tc *MockTaskConnector) CheckTaskSecret(taskID string, r *http.Request) (int, error) {
	if r.Header.Get(evergreen.TaskSecretHeader) == "" {
		return http.StatusUnauthorized, errors.New("Not authorized")
	}
	return http.StatusOK, nil
}

func (tc *MockTaskConnector) GetManifestByTask(taskId string) (*manifest.Manifest, error) {
	for _, t := range tc.CachedTasks {
		if t.Id == taskId {
			for _, m := range tc.Manifests {
				if m.Id == t.Version {
					return &m, nil
				}
			}
			return nil, errors.Errorf("no manifest found for version '%s", t.Version)
		}
	}
	return nil, errors.Errorf("task '%s' not found", taskId)
}

func (tc *MockTaskConnector) FindTasksByVersion(string, TaskFilterOptions) ([]task.Task, int, error) {
	return nil, 0, nil
}
