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
	taskIDsToSchedule, taskErrors := getSchedulableTaskIDs(selectedTasks)
	result := &BulkTaskOperationResult{
		ModifiedTasks: []*restModel.APITask{},
		TaskErrors:    taskErrors,
	}
	if len(taskIDsToSchedule) == 0 {
		return result, nil
	}
	if len(taskIDsToSchedule) > maxTasksToModify {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("cannot schedule more than %d tasks at once, but the selection contains %d schedulable tasks; narrow the selection", maxTasksToModify, len(taskIDsToSchedule)))
	}
	tasksToSchedule, err := findAllTasksByIds(ctx, taskIDsToSchedule...)
	if err != nil {
		return nil, err
	}
	logURL := evergreen.GetEnvironment().Settings().Ui.LogUrl
	scheduled, err := setTasksScheduled(ctx, logURL, true, tasksToSchedule)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("scheduling tasks: %s", err.Error()))
	}
	result.ModifiedTasks = scheduled
	return result, nil
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
