package annotations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestExecutions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.NewEnvironment(ctx, t)
	assert.NoError(t, db.Clear(Collection))
	annotations := []TaskAnnotation{
		{
			TaskId:        "t1",
			TaskExecution: 2,
			Note:          &Note{Message: "this is a note"},
		},
		{
			TaskId:        "t1",
			TaskExecution: 1,
			Note:          &Note{Message: "another note"},
		},
		{
			TaskId:        "t2",
			TaskExecution: 0,
			Note:          &Note{Message: "this is the wrong task"},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Upsert())
	}

	annotations, err := FindByTaskIds([]string{"t1", "t2"})
	assert.NoError(t, err)
	assert.Len(t, annotations, 3)
	assert.Len(t, GetLatestExecutions(annotations), 2)
}

func TestAddIssueToAnnotation(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	issue := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "annie.black"))

	annotation, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "annie.black", annotation.Issues[0].Source.Author)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)

	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "not.annie.black"))
	annotation, err = FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.Issues, 2)
	assert.NotNil(t, annotation.Issues[1].Source)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Equal(t, "not.annie.black", annotation.Issues[1].Source.Author)
}

func TestRemoveIssueFromAnnotation(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "annie.black"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "not.annie.black"}}
	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", Issues: []IssueLink{issue1, issue2}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, RemoveIssueFromAnnotation("t1", 0, issue1))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "not.annie.black", annotationFromDB.Issues[0].Source.Author)
}

func TestAddTaskLinkToAnnotation(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	taskLink := TaskLink{URL: "https://issuelink.com", Text: "Hello World"}
	assert.NoError(t, AddTaskLinkToAnnotation("t1", 0, taskLink))

	annotation, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.TaskLinks, 1)
	assert.Equal(t, "Hello World", annotation.TaskLinks[0].Text)
	assert.Equal(t, "https://issuelink.com", annotation.TaskLinks[0].URL)

	taskLink.URL = "https://issuelink.com/2"
	assert.NoError(t, AddTaskLinkToAnnotation("t1", 0, taskLink))
	annotation, err = FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.TaskLinks, 2)
	assert.Equal(t, "https://issuelink.com/2", annotation.TaskLinks[1].URL)
}

func TestRemoveTaskLinkFromAnnotation(t *testing.T) {
	taskLink1 := TaskLink{URL: "https://issuelink.com", Text: "Hello World 1"}
	taskLink2 := TaskLink{URL: "https://issuelink.com", Text: "Hello World 2"}
	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", TaskLinks: []TaskLink{taskLink1, taskLink2}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, RemoveTaskLinkFromAnnotation("t1", 0, taskLink1))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.TaskLinks, 1)
}

func TestAddSuspectedIssueToAnnotation(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	issue := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234"}
	assert.NoError(t, AddSuspectedIssueToAnnotation("t1", 0, issue, "annie.black"))

	annotation, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, UIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "annie.black", annotation.SuspectedIssues[0].Source.Author)

	assert.NoError(t, AddSuspectedIssueToAnnotation("t1", 0, issue, "not.annie.black"))
	annotation, err = FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.SuspectedIssues, 2)
	assert.NotNil(t, annotation.SuspectedIssues[1].Source)
	assert.Equal(t, "not.annie.black", annotation.SuspectedIssues[1].Source.Author)
}

func TestRemoveSuspectedIssueFromAnnotation(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "annie.black"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "not.annie.black"}}
	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", SuspectedIssues: []IssueLink{issue1, issue2}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, RemoveSuspectedIssueFromAnnotation("t1", 0, issue1))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.SuspectedIssues, 1)
	assert.Equal(t, "not.annie.black", annotationFromDB.SuspectedIssues[0].Source.Author)
}

func TestMoveIssueToSuspectedIssue(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "this will be overridden"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &Source{Author: "evergreen user"}}
	issue3 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &Source{Author: "different user"}}
	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", Issues: []IssueLink{issue1, issue2}, SuspectedIssues: []IssueLink{issue3}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, MoveIssueToSuspectedIssue(a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)

	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.Issues[0].Source.Author)
	require.Len(t, annotationFromDB.SuspectedIssues, 2)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
}

func TestMoveSuspectedIssueToIssue(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "this will be overridden"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &Source{Author: "evergreen user"}}
	issue3 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &Source{Author: "different user"}}

	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", SuspectedIssues: []IssueLink{issue1, issue2}, Issues: []IssueLink{issue3}}
	assert.NoError(t, a.Upsert())

	assert.NoError(t, MoveSuspectedIssueToIssue(a.TaskId, a.TaskExecution, issue1, "someone new"))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.SuspectedIssues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.SuspectedIssues[0].Source.Author)
	require.Len(t, annotationFromDB.Issues, 2)
	assert.Equal(t, "different user", annotationFromDB.Issues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.Issues[1].Source.Author)
}

func TestPatchIssue(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", ConfidenceScore: float64(91.23)}
	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue1, "bynn.lee"))
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345"}
	a := TaskAnnotation{TaskId: "t1", TaskExecution: 0, SuspectedIssues: []IssueLink{issue2}}
	assert.NoError(t, PatchAnnotation(&a, "not bynn", true))

	annotation, err := FindOneByTaskIdAndExecution(a.TaskId, a.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "bynn.lee", annotation.Issues[0].Source.Author)
	assert.Equal(t, "EVG-1234", annotation.Issues[0].IssueKey)
	assert.Equal(t, float64(91.23), annotation.Issues[0].ConfidenceScore)
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "not bynn", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-2345", annotation.SuspectedIssues[0].IssueKey)

	issue3 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456"}
	insert := TaskAnnotation{TaskId: "t1", TaskExecution: 1, SuspectedIssues: []IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(&insert, "insert", true))
	annotation, err = FindOneByTaskIdAndExecution(insert.TaskId, insert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "insert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)

	upsert := TaskAnnotation{TaskId: "t1", TaskExecution: 2, Note: &Note{Message: "should work"}, SuspectedIssues: []IssueLink{issue3}}
	assert.NoError(t, PatchAnnotation(&upsert, "upsert", true))
	annotation, err = FindOneByTaskIdAndExecution(upsert.TaskId, upsert.TaskExecution)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.SuspectedIssues, 1)
	assert.NotNil(t, annotation.SuspectedIssues[0].Source)
	assert.Equal(t, APIRequester, annotation.SuspectedIssues[0].Source.Requester)
	assert.Equal(t, "upsert", annotation.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "EVG-3456", annotation.SuspectedIssues[0].IssueKey)
	assert.NotNil(t, annotation.Note)
	assert.Equal(t, "should work", annotation.Note.Message)

	badInsert := TaskAnnotation{TaskId: "t1", TaskExecution: 1, Note: &Note{Message: "shouldn't work"}}
	assert.Error(t, PatchAnnotation(&badInsert, "error out ", true))

	badInsert2 := TaskAnnotation{TaskId: "t1", TaskExecution: 1, Metadata: &birch.Document{}}
	assert.Error(t, PatchAnnotation(&badInsert2, "error out ", false))
}
