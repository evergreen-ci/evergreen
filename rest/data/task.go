package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// FindTasksByBuildId uses the service layer's task type to query the backing database for a
// list of task that matches buildId. It accepts the startTaskId and a limit
// to allow for pagination of the queries. It returns results sorted by taskId.
func FindTasksByBuildId(buildId, taskId, status string, limit int, sortDir int) ([]task.Task, error) {
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

func FindTasksByProjectAndCommit(project, commitHash, taskId, status string, limit int) ([]task.Task, error) {
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

// SetTaskActivated changes the priority value of a task using a call to the
// service layer function.
func SetTaskActivated(taskId, user string, activated bool) error {
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

// ResetTask sets the task to be in an unexecuted state and prepares it to be run again.
// If given an execution task, marks the display task for reset.
func ResetTask(taskId, username string) error {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return errors.Wrapf(err, "problem finding task '%s'", t)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", t.Id)
	}
	return errors.Wrap(serviceModel.ResetTaskOrDisplayTask(t, username, evergreen.RESTV2Package, nil),
		"Reset task error")
}

func CheckTaskSecret(taskID string, r *http.Request) (int, error) {
	_, code, err := serviceModel.ValidateTask(taskID, true, r)
	if code == http.StatusConflict {
		return http.StatusUnauthorized, errors.New("Not authorized")
	}
	return code, errors.WithStack(err)
}

// GetManifestByTask finds the manifest corresponding to the given task.
func GetManifestByTask(taskId string) (*manifest.Manifest, error) {
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
	Statuses                       []string
	BaseStatuses                   []string
	Variants                       []string
	TaskNames                      []string
	Page                           int
	Limit                          int
	FieldsToProject                []string
	Sorts                          []task.TasksSortOrder
	IncludeExecutionTasks          bool
	IncludeBaseTasks               bool
	IncludeEmptyActivation         bool
	IncludeBuildVariantDisplayName bool
}

// FindTasksByVersion gets all tasks for a specific version
// Results can be filtered by task name, variant name and status in addition to being paginated and limited
func FindTasksByVersion(versionID string, opts TaskFilterOptions) ([]task.Task, int, error) {
	getTaskByVersionOpts := task.GetTasksByVersionOptions{
		Statuses:                       opts.Statuses,
		BaseStatuses:                   opts.BaseStatuses,
		Variants:                       opts.Variants,
		TaskNames:                      opts.TaskNames,
		Page:                           opts.Page,
		Limit:                          opts.Limit,
		FieldsToProject:                opts.FieldsToProject,
		Sorts:                          opts.Sorts,
		IncludeExecutionTasks:          opts.IncludeExecutionTasks,
		IncludeBaseTasks:               opts.IncludeBaseTasks,
		IncludeEmptyActivation:         opts.IncludeEmptyActivation,
		IncludeBuildVariantDisplayName: opts.IncludeBuildVariantDisplayName,
	}
	tasks, total, err := task.GetTasksByVersion(versionID, getTaskByVersionOpts)
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}
