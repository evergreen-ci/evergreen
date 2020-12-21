package annotations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestGetLatestExecutions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.NewEnvironment(ctx, t)
	assert.NoError(t, db.Clear(Collection))
	annotations := []TaskAnnotation{
		{
			Id:            "1",
			TaskId:        "t1",
			TaskExecution: 2,
			Note:          &Note{Message: "this is a note"},
		},
		{
			Id:            "2",
			TaskId:        "t1",
			TaskExecution: 1,
			Note:          &Note{Message: "another note"},
		},
		{
			Id:            "3",
			TaskId:        "t2",
			TaskExecution: 0,
			Note:          &Note{Message: "this is the wrong task"},
		},
	}
	for _, a := range annotations {
		assert.NoError(t, a.Insert())
	}

	annotations, err := FindByTaskIds([]string{"t1", "t2"})
	assert.NoError(t, err)
	assert.Len(t, annotations, 3)
	assert.Len(t, GetLatestExecutions(annotations), 2)
}

func TestAddIssueToAnnotation(t *testing.T) {
	assert.NoError(t, db.Clear(Collection))
	issue := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234"}
	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "annie.black"))

	annotation, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.Issues, 1)
	assert.NotNil(t, annotation.Issues[0].Source)
	assert.Equal(t, UIRequester, annotation.Issues[0].Source.Requester)
	assert.Equal(t, "annie.black", annotation.Issues[0].Source.Author)

	assert.NoError(t, AddIssueToAnnotation("t1", 0, issue, "not.annie.black"))
	annotation, err = FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.Issues, 2)
	assert.NotNil(t, annotation.Issues[1].Source)
	assert.Equal(t, "not.annie.black", annotation.Issues[1].Source.Author)
}

func TestRemoveIssueFromAnnotation(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "annie.black"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "not.annie.black"}}
	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{TaskId: "t1", Issues: []IssueLink{issue1, issue2}}
	assert.NoError(t, a.Insert())

	assert.NoError(t, RemoveIssueFromAnnotation("t1", 0, issue1))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "not.annie.black", annotationFromDB.Issues[0].Source.Author)
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
	assert.NoError(t, a.Insert())

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
	a := TaskAnnotation{Id: "my-annotation", TaskId: "t1", Issues: []IssueLink{issue1, issue2}, SuspectedIssues: []IssueLink{issue3}}
	assert.NoError(t, a.Insert())

	assert.NoError(t, MoveIssueToSuspectedIssue("my-annotation", issue1, "someone new"))
	annotationFromDB, err := FindByID("my-annotation")
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.Issues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.Issues[0].Source.Author)
	assert.Len(t, annotationFromDB.SuspectedIssues, 2)
	assert.Equal(t, "different user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.SuspectedIssues[1].Source.Author)
}

func TestMoveSuspectedIssueToIssue(t *testing.T) {
	issue1 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &Source{Author: "this will be overridden"}}
	issue2 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-2345", Source: &Source{Author: "evergreen user"}}
	issue3 := IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-3456", Source: &Source{Author: "different user"}}

	assert.NoError(t, db.Clear(Collection))
	a := TaskAnnotation{Id: "my-annotation", TaskId: "t1", SuspectedIssues: []IssueLink{issue1, issue2}, Issues: []IssueLink{issue3}}
	assert.NoError(t, a.Insert())

	assert.NoError(t, MoveSuspectedIssueToIssue("my-annotation", issue1, "someone new"))
	annotationFromDB, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotationFromDB)
	assert.Len(t, annotationFromDB.SuspectedIssues, 1)
	assert.Equal(t, "evergreen user", annotationFromDB.SuspectedIssues[0].Source.Author)
	assert.Len(t, annotationFromDB.Issues, 2)
	assert.Equal(t, "different user", annotationFromDB.Issues[0].Source.Author)
	assert.Equal(t, "someone new", annotationFromDB.Issues[1].Source.Author)
}

func TestValidateIssueURL(t *testing.T) {
	issue := IssueLink{URL: "issuelink.com"}
	err := ValidateIssueURL(issue.URL)
	assert.Contains(t, err.Error(), "error parsing request uri 'issuelink.com'")

	issue.URL = "http://issuelink.com/"
	err = ValidateIssueURL(issue.URL)
	assert.NoError(t, err)

	issue.URL = "http://issuelinkcom/ticket"
	err = ValidateIssueURL(issue.URL)
	assert.Contains(t, err.Error(), "issue url 'http://issuelinkcom/ticket' must have a domain and extension")

	issue.URL = "https://issuelink.com/browse/ticket"
	err = ValidateIssueURL(issue.URL)
	assert.NoError(t, err)

	issue.URL = "https://"
	err = ValidateIssueURL(issue.URL)
	assert.Contains(t, err.Error(), "issue url 'https://' must have a host name")

	issue.URL = "vscode://issuelink.com"
	err = ValidateIssueURL(issue.URL)
	assert.Contains(t, err.Error(), "issue url 'vscode://issuelink.com' scheme 'vscode' should either be 'http' or 'https'")
}
