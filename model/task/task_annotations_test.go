package task

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddIssueToAnnotation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(annotations.Collection, Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert(t.Context()))
	issue := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation(ctx, "t1", 0, issue, "annie.black"))

	annotation, err := annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, "", annotation.Id)
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, annotations.UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "annie.black", annotation.Issues[0].Source.Author)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)

	assert.NoError(t, AddIssueToAnnotation(ctx, "t1", 0, issue, "not.annie.black"))
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.Issues, 2)
	assert.NotNil(t, annotation.Issues[1].Source)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Equal(t, "not.annie.black", annotation.Issues[1].Source.Author)
	dbTask, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)
	assert.Equal(t, evergreen.TaskKnownIssue, dbTask.DisplayStatusCache)
}

func TestRemoveIssueFromAnnotation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "annie.black"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "not.annie.black"}}
	assert.NoError(t, db.ClearCollections(annotations.Collection, Collection))
	a := annotations.TaskAnnotation{TaskId: "t1", Issues: []annotations.IssueLink{issue1, issue2}}
	assert.NoError(t, a.Upsert(t.Context()))
	task := Task{Id: "t1", HasAnnotations: true, Status: evergreen.TaskFailed, DisplayStatusCache: evergreen.TaskKnownIssue}
	assert.NoError(t, task.Insert(t.Context()))

	// Task should still have annotations key set after first issue is removed
	assert.NoError(t, RemoveIssueFromAnnotation(ctx, "t1", 0, issue1))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "not.annie.black", annotationFromDB.Issues[0].Source.Author)
	dbTask, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)
	assert.Equal(t, evergreen.TaskKnownIssue, dbTask.DisplayStatusCache)

	// Removing the second issue should mark the task as no longer having annotations
	assert.NoError(t, RemoveIssueFromAnnotation(ctx, "t1", 0, issue2))
	annotationFromDB, err = annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Empty(t, annotationFromDB.Issues)
	dbTask, err = FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.False(t, dbTask.HasAnnotations)
	assert.Equal(t, evergreen.TaskFailed, dbTask.DisplayStatusCache)
}

func TestMoveIssueToSuspectedIssue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "this will be overridden"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &annotations.Source{Author: "evergreen user"}}
	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &annotations.Source{Author: "different user"}}
	assert.NoError(t, db.ClearCollections(annotations.Collection, Collection))
	a := annotations.TaskAnnotation{TaskId: "t1", Issues: []annotations.IssueLink{issue1, issue2}, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, a.Upsert(t.Context()))
	task := Task{Id: "t1", HasAnnotations: true}
	assert.NoError(t, task.Insert(t.Context()))

	assert.NoError(t, MoveIssueToSuspectedIssue(ctx, a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution(t.Context(), a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	// Task should still have annotations key set after first issue is removed
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.Issues[0].Source.Author)
	require.Len(t, annotationFromDB.SuspectedIssues, 2)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
	dbTask, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)

	// Removing the second issue should mark the task as no longer having annotations
	assert.NoError(t, MoveIssueToSuspectedIssue(ctx, a.TaskId, a.TaskExecution, issue2, "someone else new"))
	annotationFromDB, err = annotations.FindOneByTaskIdAndExecution(t.Context(), a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Empty(t, annotationFromDB.Issues)
	require.Len(t, annotationFromDB.SuspectedIssues, 3)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
	assert.Equal(t, "someone else new", annotationFromDB.SuspectedIssues[2].Source.Author)
	dbTask, err = FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.False(t, dbTask.HasAnnotations)
}

func TestMoveSuspectedIssueToIssue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "this will be overridden"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &annotations.Source{Author: "evergreen user"}}
	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &annotations.Source{Author: "different user"}}

	assert.NoError(t, db.ClearCollections(annotations.Collection, Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert(t.Context()))
	a := annotations.TaskAnnotation{TaskId: "t1", SuspectedIssues: []annotations.IssueLink{issue1, issue2}, Issues: []annotations.IssueLink{issue3}}
	assert.NoError(t, a.Upsert(t.Context()))

	assert.NoError(t, MoveSuspectedIssueToIssue(ctx, a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution(t.Context(), "t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.SuspectedIssues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.SuspectedIssues[0].Source.Author)
	require.Len(t, annotationFromDB.Issues, 2)
	assert.Equal(t, "different user", annotationFromDB.Issues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.Issues[1].Source.Author)
	dbTask, err := FindOneId(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)
	assert.Equal(t, evergreen.TaskKnownIssue, dbTask.DisplayStatusCache)
}

func TestPatchIssue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(annotations.Collection, Collection))
	t1 := Task{Id: "t1"}
	assert.NoError(t, t1.Insert(t.Context()))
	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation(ctx, "t1", 0, issue1, "bynn.lee"))
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345"}
	a := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue2}}
	assert.NoError(t, PatchAnnotation(ctx, &a, "not bynn", true))

	annotation, err := annotations.FindOneByTaskIdAndExecution(t.Context(), a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, "", annotation.Id)
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, annotations.UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "bynn.lee", annotation.Issues[0].Source.Author)
	assert.Equal(t, "EVG-1234", annotation.Issues[0].IssueKey)
	//nolint:testifylint // We expect it to be exactly equal.
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "not bynn", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-2345", annotation.SuspectedIssues[0].IssueKey)

	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456"}
	insert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(ctx, &insert, "insert", true))
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), insert.TaskId, insert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, "", annotation.Id)
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "insert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)

	upsert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 2, Note: &annotations.Note{Message: "should work"}, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(ctx, &upsert, "upsert", true))
	annotation, err = annotations.FindOneByTaskIdAndExecution(t.Context(), upsert.TaskId, upsert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, "", annotation.Id)
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "upsert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)
	assert.NotNil(t, annotation.Note)
	assert.Equal(t, "should work", annotation.Note.Message)

	badInsert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, Note: &annotations.Note{Message: "shouldn't work"}}
	assert.Error(t, PatchAnnotation(ctx, &badInsert, "error out", true))

	badInsert2 := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, Metadata: &birch.Document{}}
	assert.Error(t, PatchAnnotation(ctx, &badInsert2, "error out", false))

	// Check that HasAnnotations field is correctly in sync when patching issues array.
	t2 := Task{Id: "t2"}
	assert.NoError(t, t2.Insert(t.Context()))

	annotationUpdate := annotations.TaskAnnotation{TaskId: "t2", TaskExecution: 0, Issues: []annotations.IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(ctx, &annotationUpdate, "jane.smith", true))

	foundTask, err := FindOneId(ctx, t2.Id)
	require.NoError(t, err)
	require.NotNil(t, foundTask)
	assert.True(t, foundTask.HasAnnotations)

	annotationUpdate = annotations.TaskAnnotation{TaskId: "t2", TaskExecution: 0, Issues: []annotations.IssueLink{}}
	assert.NoError(t, PatchAnnotation(ctx, &annotationUpdate, "jane.smith", true))

	foundTask, err = FindOneId(ctx, t2.Id)
	require.NoError(t, err)
	require.NotNil(t, foundTask)
	assert.False(t, foundTask.HasAnnotations)
}

func TestUpdateHasAnnotationsWithArchivedTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, OldCollection, annotations.Collection))

	task := Task{
		Id:        "test_task_archived",
		Execution: 0,
		Status:    evergreen.TaskFailed,
	}
	assert.NoError(t, task.Insert(t.Context()))

	// Verify the task does not have annotations.
	dbTask, err := FindOneId(t.Context(), task.Id)
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.False(t, dbTask.HasAnnotations)

	// Test UpdateHasAnnotations works for current task
	assert.NoError(t, UpdateHasAnnotations(t.Context(), task.Id, 0, true))
	dbTask, err = FindOneId(t.Context(), task.Id)
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)

	// Archive the task (simulating a restart) - use the updated task from the database
	assert.NoError(t, dbTask.Archive(t.Context()))

	// Verify the task was archived and new execution was created
	currentTask, err := FindOneId(t.Context(), task.Id)
	require.NoError(t, err)
	require.NotNil(t, currentTask)
	assert.Equal(t, 1, currentTask.Execution)
	assert.False(t, currentTask.HasAnnotations) // New execution should not have annotations

	// Verify archived task exists
	archivedTask, err := FindOneOldByIdAndExecution(t.Context(), task.Id, 0)
	require.NoError(t, err)
	require.NotNil(t, archivedTask)
	assert.Equal(t, 0, archivedTask.Execution)
	assert.True(t, archivedTask.Archived)
	assert.True(t, archivedTask.HasAnnotations) // Should still have annotations from before archival

	// Test that UpdateHasAnnotations works for archived task
	// This should find the task in the old_tasks collection and update it
	err = UpdateHasAnnotations(t.Context(), task.Id, 0, false)
	assert.NoError(t, err, "UpdateHasAnnotations should succeed for archived task")

	// Verify the archived task was updated
	archivedTask, err = FindOneOldByIdAndExecution(t.Context(), task.Id, 0)
	require.NoError(t, err)
	require.NotNil(t, archivedTask)
	assert.False(t, archivedTask.HasAnnotations)

	// Test that UpdateHasAnnotations still works for current task.
	require.NoError(t, UpdateHasAnnotations(t.Context(), task.Id, 1, true))
	currentTask, err = FindOneId(t.Context(), task.Id)
	require.NoError(t, err)
	require.NotNil(t, currentTask)
	assert.True(t, currentTask.HasAnnotations)

	// Test updating the archived task back to having annotations.
	err = UpdateHasAnnotations(t.Context(), task.Id, 0, true)
	require.NoError(t, err, "UpdateHasAnnotations should succeed when setting back to true")
	archivedTask, err = FindOneOldByIdAndExecution(t.Context(), task.Id, 0)
	require.NoError(t, err)
	require.NotNil(t, archivedTask)
	assert.True(t, archivedTask.HasAnnotations)
}
