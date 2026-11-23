package graphql

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection))
	})

	v := model.Version{
		Id:         "v1",
		Identifier: "proj",
		Requester:  evergreen.PatchVersionRequester,
	}
	require.NoError(t, v.Insert(t.Context()))

	makeTask := func(id, version, status string) task.Task {
		return task.Task{
			Id:                 id,
			DisplayName:        id,
			Version:            version,
			Project:            "proj",
			BuildVariant:       "bv1",
			Status:             status,
			DisplayStatusCache: status,
			ActivatedTime:      time.Now(),
			DisplayTaskId:      utility.ToStringPtr(""),
		}
	}
	tasks := []task.Task{
		makeTask("failed1", "v1", evergreen.TaskFailed),
		makeTask("failed2", "v1", evergreen.TaskFailed),
		makeTask("failed3", "v1", evergreen.TaskFailed),
		makeTask("succeeded1", "v1", evergreen.TaskSucceeded),
		makeTask("other_version", "v2", evergreen.TaskFailed),
	}
	for _, tsk := range tasks {
		require.NoError(t, tsk.Insert(t.Context()))
	}

	failedFilter := &TaskFilterOptions{Statuses: []string{evergreen.TaskFailed}}

	t.Run("TooManyTaskIDsShouldError", func(t *testing.T) {
		_, err := resolveTasksByIDs(t.Context(), make([]string, maxTasksToModify+1))
		require.Error(t, err)
		assert.ErrorContains(t, err, "cannot select more than 1000 tasks by ID, use a filter instead")
	})

	t.Run("FilterOnUnknownVersionShouldError", func(t *testing.T) {
		_, err := resolveTasksByFilter(t.Context(), "nonexistent", *failedFilter)
		require.Error(t, err)
		assert.ErrorContains(t, err, "version 'nonexistent' not found")
	})

	t.Run("TasksByIds", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			taskIDs     []string
			expectedIDs []string
		}{
			"ReturnsOnlyTheRequestedTasks": {
				taskIDs:     []string{"failed1", "succeeded1"},
				expectedIDs: []string{"failed1", "succeeded1"},
			},
			"ReturnsTasksFromDifferentVersions": {
				taskIDs:     []string{"failed1", "other_version"},
				expectedIDs: []string{"failed1", "other_version"},
			},
			"OmitsNonexistentTasks": {
				taskIDs:     []string{"failed1", "nonexistent"},
				expectedIDs: []string{"failed1"},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				resolved, err := resolveTasksByIDs(t.Context(), testCase.taskIDs)
				require.NoError(t, err)
				taskIDs := make([]string, 0, len(resolved))
				for _, t := range resolved {
					taskIDs = append(taskIDs, t.Id)
				}
				assert.ElementsMatch(t, testCase.expectedIDs, taskIDs)
			})
		}
	})

	t.Run("TasksByFilter", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			filter      *TaskFilterOptions
			expectedIDs []string
		}{
			"ReturnsAllMatchingTasksInVersion": {
				filter:      failedFilter,
				expectedIDs: []string{"failed1", "failed2", "failed3"},
			},
			"EmptyFilterReturnsAllTasksInVersion": {
				filter:      &TaskFilterOptions{},
				expectedIDs: []string{"failed1", "failed2", "failed3", "succeeded1"},
			},
			"OmitsExcludedTasks": {
				filter:      &TaskFilterOptions{Statuses: []string{evergreen.TaskFailed}, ExcludeTaskIds: []string{"failed2"}},
				expectedIDs: []string{"failed1", "failed3"},
			},
			"IgnoresLimitAndPage": {
				filter:      &TaskFilterOptions{Statuses: []string{evergreen.TaskFailed}, Limit: utility.ToIntPtr(1), Page: utility.ToIntPtr(1)},
				expectedIDs: []string{"failed1", "failed2", "failed3"},
			},
			"MatchingNothingReturnsNoTasks": {
				filter: &TaskFilterOptions{Statuses: []string{evergreen.TaskStarted}},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				resolved, err := resolveTasksByFilter(t.Context(), "v1", *testCase.filter)
				require.NoError(t, err)
				taskIDs := make([]string, 0, len(resolved))
				for _, t := range resolved {
					taskIDs = append(taskIDs, t.Id)
				}
				assert.ElementsMatch(t, testCase.expectedIDs, taskIDs)
			})
		}
	})
}

func TestGetTaskIDsToSchedule(t *testing.T) {
	regular := task.Task{Id: "regular", DisplayTaskId: utility.ToStringPtr("")}
	display := task.Task{Id: "display", DisplayOnly: true, DisplayTaskId: utility.ToStringPtr("")}
	exec1 := task.Task{Id: "exec1", DisplayTaskId: utility.ToStringPtr("display")}
	exec2 := task.Task{Id: "exec2", DisplayTaskId: utility.ToStringPtr("display")}

	for testName, testCase := range map[string]struct {
		tasks       []task.Task
		expectedIDs []string
	}{
		"RegularAndDisplayTasksAreKept": {
			tasks:       []task.Task{regular, display},
			expectedIDs: []string{"regular", "display"},
		},
		"ExecutionTaskIsReplacedByItsDisplayTask": {
			tasks:       []task.Task{exec1},
			expectedIDs: []string{"display"},
		},
		"SiblingExecutionTasksCollapseToOneDisplayTask": {
			tasks:       []task.Task{exec1, exec2},
			expectedIDs: []string{"display"},
		},
		"DisplayTaskAlreadySelectedIsNotDuplicated": {
			tasks:       []task.Task{display, exec1},
			expectedIDs: []string{"display"},
		},
		"DuplicateTasksAreDeduplicated": {
			tasks:       []task.Task{regular, regular},
			expectedIDs: []string{"regular"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.ElementsMatch(t, testCase.expectedIDs, getTaskIDsToSchedule(testCase.tasks))
		})
	}
}

func TestGetSchedulableTasks(t *testing.T) {
	unscheduled := task.Task{Id: "unscheduled", Status: evergreen.TaskUndispatched, Activated: false}
	scheduled := task.Task{Id: "scheduled", Status: evergreen.TaskUndispatched, Activated: true}
	running := task.Task{Id: "running", Status: evergreen.TaskStarted, Activated: true}
	finished := task.Task{Id: "finished", Status: evergreen.TaskFailed, Activated: true}
	mergeQueue := task.Task{Id: "mergeQueue", Status: evergreen.TaskUndispatched, Activated: false, Requester: evergreen.GithubMergeRequester}
	unscheduledDisplay := task.Task{Id: "display", DisplayOnly: true, Status: evergreen.TaskUndispatched}
	scheduledDisplay := task.Task{Id: "scheduledDisplay", DisplayOnly: true, Status: evergreen.TaskUndispatched, Activated: true}

	for testName, testCase := range map[string]struct {
		tasks            []task.Task
		expectedIDs      []string
		expectedErrorIDs []string
	}{
		"UnscheduledTaskIsSchedulable": {
			tasks:       []task.Task{unscheduled},
			expectedIDs: []string{"unscheduled"},
		},
		"ScheduledRunningAndFinishedTasksAreNotSchedulable": {
			tasks:            []task.Task{scheduled, running, finished},
			expectedErrorIDs: []string{"scheduled", "running", "finished"},
		},
		"UnscheduledMergeQueueTaskIsNotSchedulable": {
			tasks:            []task.Task{mergeQueue},
			expectedErrorIDs: []string{"mergeQueue"},
		},
		"UnscheduledDisplayTaskIsSchedulable": {
			tasks:       []task.Task{unscheduledDisplay},
			expectedIDs: []string{"display"},
		},
		"ScheduledDisplayTaskIsNotSchedulable": {
			tasks:            []task.Task{scheduledDisplay},
			expectedErrorIDs: []string{"scheduledDisplay"},
		},
		"MixedTasksAreSplitBySchedulability": {
			tasks:            []task.Task{unscheduled, finished, unscheduledDisplay, scheduledDisplay},
			expectedIDs:      []string{"unscheduled", "display"},
			expectedErrorIDs: []string{"finished", "scheduledDisplay"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			schedulable, taskErrors := getSchedulableTasks(testCase.tasks)
			ids := []string{}
			for _, s := range schedulable {
				ids = append(ids, s.Id)
			}
			errorIDs := []string{}
			for _, e := range taskErrors {
				errorIDs = append(errorIDs, e.TaskID)
			}
			assert.ElementsMatch(t, testCase.expectedIDs, ids)
			assert.ElementsMatch(t, testCase.expectedErrorIDs, errorIDs)
		})
	}
}
