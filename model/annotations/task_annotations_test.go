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
	taskAnnotations := []TaskAnnotation{
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
	for _, a := range taskAnnotations {
		assert.NoError(t, a.Upsert())
	}

	taskAnnotations, err := FindByTaskIds([]string{"t1", "t2"})
	assert.NoError(t, err)
	assert.Len(t, taskAnnotations, 3)
	assert.Len(t, GetLatestExecutions(taskAnnotations), 2)
}

func TestSetAnnotationMetadataLinks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.Clear(Collection))
	taskLink := MetadataLink{URL: "https://issuelink.com", Text: "Hello World"}
	assert.NoError(t, SetAnnotationMetadataLinks(ctx, "t1", 0, "usr", taskLink))

	annotation, err := FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.NotEqual(t, annotation.Id, "")
	assert.Len(t, annotation.MetadataLinks, 1)
	assert.Equal(t, "Hello World", annotation.MetadataLinks[0].Text)
	assert.Equal(t, "https://issuelink.com", annotation.MetadataLinks[0].URL)
	assert.NotNil(t, annotation.MetadataLinks[0].Source)
	assert.Equal(t, "usr", annotation.MetadataLinks[0].Source.Author)

	taskLink.URL = "https://issuelink.com/2"
	assert.NoError(t, SetAnnotationMetadataLinks(ctx, "t1", 0, "usr", taskLink))
	annotation, err = FindOneByTaskIdAndExecution("t1", 0)
	assert.NoError(t, err)
	assert.NotNil(t, annotation)
	assert.Len(t, annotation.MetadataLinks, 1)
	assert.Equal(t, "https://issuelink.com/2", annotation.MetadataLinks[0].URL)
	assert.NotNil(t, annotation.MetadataLinks[0].Source)
	assert.Equal(t, "usr", annotation.MetadataLinks[0].Source.Author)
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
