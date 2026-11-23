package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
)

const (
	maxTasksToModify = 1000
)

var taskSelectionFields = []string{
	task.IdKey,
	task.StatusKey,
	task.ActivatedKey,
	task.RequesterKey,
	task.DisplayTaskIdKey,
}

// resolveTaskSelection returns the tasks in the version that the selection refers to, along with any task errors encountered.
func resolveTaskSelection(ctx context.Context, versionID string, selection TaskSelection) ([]task.Task, []*TaskError, error) {
	hasTaskIDs := selection.TaskIds != nil
	hasFilters := selection.Filter != nil
	if hasTaskIDs == hasFilters {
		return nil, nil, InputValidationError.Send(ctx, "exactly one of 'taskIds' or 'filter' must be set in a task selection")
	}
	if hasTaskIDs {
		if len(selection.TaskIds) > maxTasksToModify {
			return nil, nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot select more than %d tasks by ID, use a filter instead", maxTasksToModify))
		}
		return resolveTasksByIDs(ctx, versionID, selection.TaskIds)
	}
	return resolveTasksByFilter(ctx, versionID, selection.Filter)
}

func resolveTasksByIDs(ctx context.Context, versionID string, taskIDs []string) ([]task.Task, []*TaskError, error) {
	tasks, err := task.FindAllByIdsAndVersion(ctx, taskIDs, versionID, taskSelectionFields...)
	if err != nil {
		return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("finding tasks in version '%s': %s", versionID, err.Error()))
	}
	found := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		found[t.Id] = true
	}
	taskErrors := []*TaskError{}
	for _, id := range taskIDs {
		if found[id] {
			continue
		}
		found[id] = true
		taskErrors = append(taskErrors, &TaskError{TaskID: id, Reason: fmt.Sprintf("task not found in version '%s'", versionID)})
	}
	return tasks, taskErrors, nil
}

func resolveTasksByFilter(ctx context.Context, versionID string, f *TaskFilterOptions) ([]task.Task, []*TaskError, error) {
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}

	opts, err := getTasksByVersionOptions(ctx, v, *f)
	if err != nil {
		return nil, nil, err
	}
	opts.FieldsToProject = taskSelectionFields

	tasks, _, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version '%s': %s", versionID, err.Error()))
	}
	return tasks, []*TaskError{}, nil
}

// getSchedulableTaskIDs returns the unique IDs of the tasks to schedule and an error for each given task that can't be
// scheduled. Execution tasks can't be scheduled individually, so they're replaced by their display task.
func getSchedulableTaskIDs(tasks []task.Task) ([]string, []*TaskError) {
	seen := make(map[string]bool, len(tasks))
	rejected := make(map[string]bool, len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	taskErrors := []*TaskError{}
	for _, t := range tasks {
		id := t.Id
		displayTaskID := utility.FromStringPtr(t.DisplayTaskId)
		if displayTaskID != "" {
			id = displayTaskID
		}

		if displayTaskID == "" && !t.IsUnscheduled() {
			rejected[id] = true
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "task is already scheduled or has already run"})
			continue
		}
		if evergreen.IsGithubMergeQueueRequester(t.Requester) {
			rejected[id] = true
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "merge queue tasks cannot be manually scheduled"})
			continue
		}
		if !seen[id] {
			seen[id] = true
			taskIDs = append(taskIDs, id)
		}
	}

	schedulableIDs := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		if !rejected[id] {
			schedulableIDs = append(schedulableIDs, id)
		}
	}
	return schedulableIDs, taskErrors
}
