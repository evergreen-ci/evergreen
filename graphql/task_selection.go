package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
)

// resolveTaskSelection returns the tasks in the version that the selection refers to, along with any explicitly
// requested tasks that couldn't be found in the version.
func resolveTaskSelection(ctx context.Context, versionID string, selection TaskSelection) ([]task.Task, []*TaskError, error) {
	hasTaskIDs := selection.TaskIds != nil
	hasFilters := selection.Filter != nil
	if hasTaskIDs == hasFilters {
		return nil, nil, InputValidationError.Send(ctx, "exactly one of 'taskIds' or 'filter' must be set in a task selection")
	}
	if hasTaskIDs {
		return resolveTasksByIDs(ctx, versionID, selection.TaskIds)
	}
	return resolveTasksByFilter(ctx, versionID, selection.Filter)
}

// taskSelectionFields are the only task fields loaded when resolving a task selection.
var taskSelectionFields = []string{
	task.IdKey,
	task.StatusKey,
	task.ActivatedKey,
	task.RequesterKey,
	task.DisplayTaskIdKey,
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

func resolveTasksByFilter(ctx context.Context, versionID string, f *TaskSelectionFilter) ([]task.Task, []*TaskError, error) {
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}
	opts := task.GetTasksByVersionOptions{
		Statuses:     getValidTaskStatusesFilter(f.Statuses),
		BaseStatuses: getValidTaskStatusesFilter(f.BaseStatuses),
		Variants:     []string{utility.FromStringPtr(f.Variant)},
		TaskNames:    []string{utility.FromStringPtr(f.TaskName)},
		// If the version is mainline, include never-activated tasks regardless of the user's preference.
		IncludeNeverActivatedTasks: utility.FromBoolPtr(f.IncludeNeverActivatedTasks) || !evergreen.IsPatchRequester(v.Requester),
		FieldsToProject:            taskSelectionFields,
	}
	// Base version is only needed if filtering on base statuses.
	if len(opts.BaseStatuses) > 0 {
		baseVersion, err := v.FindBaseVersion(ctx, false)
		if err != nil {
			return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("finding base version for version '%s': %s", v.Id, err.Error()))
		}
		if baseVersion != nil {
			opts.BaseVersionID = baseVersion.Id
		}
	}
	matched, _, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version '%s': %s", versionID, err.Error()))
	}

	// A user can apply a filter but then unselect specific tasks. Here we remove any tasks the user has excluded.
	excluded := make(map[string]bool, len(f.ExcludeTaskIds))
	for _, id := range f.ExcludeTaskIds {
		excluded[id] = true
	}
	tasks := make([]task.Task, 0, len(matched))
	for _, t := range matched {
		if !excluded[t.Id] {
			tasks = append(tasks, t)
		}
	}
	return tasks, []*TaskError{}, nil
}

// getSchedulableTaskIDs returns the unique IDs of the tasks to schedule and an error for each given task that can't be
// scheduled.
func getSchedulableTaskIDs(tasks []task.Task) ([]string, []*TaskError) {
	seen := make(map[string]bool, len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	taskErrors := []*TaskError{}
	for _, t := range tasks {
		if evergreen.IsGithubMergeQueueRequester(t.Requester) {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "merge queue tasks cannot be manually scheduled"})
			continue
		}
		if !t.IsUnscheduled() {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "task is already scheduled or has already run"})
			continue
		}
		id := t.Id
		// Use display task ID if it's an execution task.
		if displayTaskID := utility.FromStringPtr(t.DisplayTaskId); displayTaskID != "" {
			id = displayTaskID
		}
		if !seen[id] {
			seen[id] = true
			taskIDs = append(taskIDs, id)
		}
	}
	return taskIDs, taskErrors
}
