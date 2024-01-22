package task

import (
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddIssueToAnnotation(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.TaskAnnotationsCollection, Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert())
	issue := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "annie.black"))

	annotation, err := annotations.FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, annotations.UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "annie.black", annotation.Issues[0].Source.Author)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)

	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "not.annie.black"))
	annotation, err = annotations.FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.Issues, 2)
	assert.NotNil(t, annotation.Issues[1].Source)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Equal(t, "not.annie.black", annotation.Issues[1].Source.Author)
	dbTask, err := FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)
}

func TestRemoveIssueFromAnnotation(t *testing.T) {
	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "annie.black"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "not.annie.black"}}
	assert.NoError(t, db.ClearCollections(annotations.TaskAnnotationsCollection, Collection))
	a := annotations.TaskAnnotation{TaskId: "t1", Issues: []annotations.IssueLink{issue1, issue2}}
	assert.NoError(t, a.Upsert())
	task := Task{Id: "t1", HasAnnotations: true}
	assert.NoError(t, task.Insert())

	// Task should still have annotations key set after first issue is removed
	assert.NoError(t, RemoveIssueFromAnnotation("t1", 0, issue1))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "not.annie.black", annotationFromDB.Issues[0].Source.Author)
	dbTask, err := FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)

	// Removing the second issue should mark the task as no longer having annotations
	assert.NoError(t, RemoveIssueFromAnnotation("t1", 0, issue2))
	annotationFromDB, err = annotations.FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 0)
	dbTask, err = FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.False(t, dbTask.HasAnnotations)
}

func TestMoveIssueToSuspectedIssue(t *testing.T) {
	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "this will be overridden"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &annotations.Source{Author: "evergreen user"}}
	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &annotations.Source{Author: "different user"}}
	assert.NoError(t, db.ClearCollections(annotations.TaskAnnotationsCollection, Collection))
	a := annotations.TaskAnnotation{TaskId: "t1", Issues: []annotations.IssueLink{issue1, issue2}, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, a.Upsert())
	task := Task{Id: "t1", HasAnnotations: true}
	assert.NoError(t, task.Insert())

	assert.NoError(t, MoveIssueToSuspectedIssue(a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	// Task should still have annotations key set after first issue is removed
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.Issues[0].Source.Author)
	require.Len(t, annotationFromDB.SuspectedIssues, 2)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
	dbTask, err := FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)

	// Removing the second issue should mark the task as no longer having annotations
	assert.NoError(t, MoveIssueToSuspectedIssue(a.TaskId, a.TaskExecution, issue2, "someone else new"))
	annotationFromDB, err = annotations.FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 0)
	require.Len(t, annotationFromDB.SuspectedIssues, 3)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
	assert.Equal(t, "someone else new", annotationFromDB.SuspectedIssues[2].Source.Author)
	dbTask, err = FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.False(t, dbTask.HasAnnotations)
}

func TestMoveSuspectedIssueToIssue(t *testing.T) {
	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "this will be overridden"}}
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &annotations.Source{Author: "evergreen user"}}
	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &annotations.Source{Author: "different user"}}

	assert.NoError(t, db.ClearCollections(annotations.TaskAnnotationsCollection, Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert())
	a := annotations.TaskAnnotation{TaskId: "t1", SuspectedIssues: []annotations.IssueLink{issue1, issue2}, Issues: []annotations.IssueLink{issue3}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, MoveSuspectedIssueToIssue(a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := annotations.FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.SuspectedIssues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.SuspectedIssues[0].Source.Author)
	require.Len(t, annotationFromDB.Issues, 2)
	assert.Equal(t, "different user", annotationFromDB.Issues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.Issues[1].Source.Author)
	dbTask, err := FindOneId("t1")
	require.NoError(t, err)
	require.NotNil(t, dbTask)
	assert.True(t, dbTask.HasAnnotations)
}

func TestPatchIssue(t *testing.T) {
	assert.NoError(t, db.ClearCollections(annotations.TaskAnnotationsCollection, Collection))
	task := Task{Id: "t1"}
	assert.NoError(t, task.Insert())
	issue1 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue1, "bynn.lee"))
	issue2 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345"}
	a := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue2}}
	assert.NoError(t, PatchAnnotation(&a, "not bynn", true))

	annotation, err := annotations.FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, annotations.UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "bynn.lee", annotation.Issues[0].Source.Author)
	assert.Equal(t, "EVG-1234", annotation.Issues[0].IssueKey)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "not bynn", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-2345", annotation.SuspectedIssues[0].IssueKey)

	issue3 := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456"}
	insert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(&insert, "insert", true))
	annotation, err = annotations.FindOneByTaskIdAndExecution(insert.TaskId, insert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "insert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)

	upsert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 2, Note: &annotations.Note{Message: "should work"}, SuspectedIssues: []annotations.IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(&upsert, "upsert", true))
	annotation, err = annotations.FindOneByTaskIdAndExecution(upsert.TaskId, upsert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, annotations.APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "upsert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)
	assert.NotNil(t, annotation.Note)
	assert.Equal(t, "should work", annotation.Note.Message)

	badInsert := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, Note: &annotations.Note{Message: "shouldn't work"}}
	assert.Error(t, PatchAnnotation(&badInsert, "error out ", true))

	badInsert2 := annotations.TaskAnnotation{TaskId: "t1", TaskExecution: 1, Metadata: &birch.Document{}}
	assert.Error(t, PatchAnnotation(&badInsert2, "error out ", false))
}
