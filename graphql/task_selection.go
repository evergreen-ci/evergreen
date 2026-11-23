package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

const (
	maxTasksToModify = 1000
)

// resolveTasksByIDs returns the tasks with the given IDs, which may belong to different versions.
func resolveTasksByIDs(ctx context.Context, taskIDs []string) ([]task.Task, error) {
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot select more than %d tasks by ID, use a filter instead", maxTasksToModify))
	}
	return findAllTasksByIds(ctx, taskIDs...)
}

// resolveTasksByFilter returns the tasks in the version that match the filter.
func resolveTasksByFilter(ctx context.Context, versionID string, f TaskFilterOptions) ([]task.Task, error) {
	v, err := model.VersionFindOneId(ctx, versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding version '%s': %s", versionID, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("version '%s' not found", versionID))
	}

	opts, err := getTasksByVersionOptions(ctx, v, f)
	if err != nil {
		return nil, err
	}
	opts.FieldsToProject = []string{
		task.IdKey,
		task.StatusKey,
		task.ActivatedKey,
		task.RequesterKey,
		task.DisplayTaskIdKey,
	}

	tasks, _, err := task.GetTasksByVersion(ctx, versionID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting tasks for version '%s': %s", versionID, err.Error()))
	}
	return tasks, nil
}

// scheduleSelectedTasks schedules the schedulable tasks among the selected tasks and reports the rest as task errors.
func scheduleSelectedTasks(ctx context.Context, selectedTasks []task.Task) (*BulkTaskOperationResult, error) {
	result := &BulkTaskOperationResult{
		ModifiedTasks: []*restModel.APITask{},
		TaskErrors:    []*TaskError{},
	}
	taskIDs := getTaskIDsToModify(selectedTasks)
	if len(taskIDs) == 0 {
		return result, nil
	}
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot schedule more than %d tasks at once, but the selection contains %d tasks; narrow the selection", maxTasksToModify, len(taskIDs)))
	}
	tasks, err := findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}
	tasksToSchedule, taskErrors := getSchedulableTasks(tasks)
	result.TaskErrors = taskErrors
	if len(tasksToSchedule) == 0 {
		return result, nil
	}
	logURL := evergreen.GetEnvironment().Settings().Ui.LogUrl
	scheduled, err := setTasksScheduled(ctx, logURL, true, tasksToSchedule)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("scheduling tasks: %s", err.Error()))
	}
	result.ModifiedTasks = scheduled
	return result, nil
}

// getTaskIDsToSchedule returns the unique IDs of the tasks to schedule. Execution tasks can't generally be operated on.
func getTaskIDsToModify(tasks []task.Task) []string {
	seen := make(map[string]bool, len(tasks))
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		id := t.Id
		if displayTaskID := utility.FromStringPtr(t.DisplayTaskId); displayTaskID != "" {
			id = displayTaskID
		}
		if !seen[id] {
			seen[id] = true
			taskIDs = append(taskIDs, id)
		}
	}
	return taskIDs
}

// getSchedulableTasks returns the tasks that can be scheduled and an error for each task that can't be.
func getSchedulableTasks(tasks []task.Task) ([]task.Task, []*TaskError) {
	schedulable := make([]task.Task, 0, len(tasks))
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
		schedulable = append(schedulable, t)
	}
	return schedulable, taskErrors
}

// unscheduleSelectedTasks unschedules the schedulable tasks among the selected tasks and reports the rest as task errors.
func unscheduleSelectedTasks(ctx context.Context, selectedTasks []task.Task) (*BulkTaskOperationResult, error) {
	result := &BulkTaskOperationResult{
		ModifiedTasks: []*restModel.APITask{},
		TaskErrors:    []*TaskError{},
	}
	taskIDs := getTaskIDsToModify(selectedTasks)
	if len(taskIDs) == 0 {
		return result, nil
	}
	if len(taskIDs) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot schedule more than %d tasks at once, but the selection contains %d tasks; narrow the selection", maxTasksToModify, len(taskIDs)))
	}
	tasks, err := findAllTasksByIds(ctx, taskIDs...)
	if err != nil {
		return nil, err
	}
	tasksToSchedule, taskErrors := getSchedulableTasks(tasks)
	result.TaskErrors = taskErrors
	if len(tasksToSchedule) == 0 {
		return result, nil
	}
	logURL := evergreen.GetEnvironment().Settings().Ui.LogUrl
	scheduled, err := setTasksScheduled(ctx, logURL, false, tasksToSchedule)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("scheduling tasks: %s", err.Error()))
	}
	result.ModifiedTasks = scheduled
	return result, nil
}

// getSchedulableTasks returns the tasks that can be scheduled and an error for each task that can't be.
func getUnschedulableTasks(tasks []task.Task) ([]task.Task, []*TaskError) {
	unschedulable := make([]task.Task, 0, len(tasks))
	taskErrors := []*TaskError{}
	for _, t := range tasks {
		if t.Activated && t.Status == evergreen.TaskUndispatched && t.ParentPatchID == "" {
			taskErrors = append(taskErrors, &TaskError{TaskID: t.Id, Reason: "task is part of an active display task and cannot be unscheduled individually"})
			continue
		}
		unschedulable = append(unschedulable, t)
	}
	return unschedulable, taskErrors
}
