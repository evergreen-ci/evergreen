package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGithubMergePRValid(t *testing.T) {
	validMessage := GithubMergePR{
		ProjectID: "evergreen",
		Item:      "12345",
		Status:    evergreen.PatchSucceeded,
		PRs: []event.PRInfo{
			{
				Owner: "evergreen-ci",
				Repo:  "evergreen",
				Ref:   "abcdef",
				PRNum: 1,
			},
		},
	}
	assert.True(t, validMessage.Valid())

	missingProjectID := validMessage
	missingProjectID.ProjectID = ""
	assert.False(t, missingProjectID.Valid())

	prMissingInfo := validMessage
	prMissingInfo.PRs[0].Owner = ""
	assert.False(t, prMissingInfo.Valid())

	invalidMergeMethod := validMessage
	invalidMergeMethod.MergeMethod = "not a merge method"
	assert.False(t, invalidMergeMethod.Valid())
}

func TestDequeueFromCommitQueue(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	projectID := "evergreen"
	itemID := "abcdef"
	queue := &CommitQueue{
		ProjectID: projectID,
		Queue:     []CommitQueueItem{{Issue: itemID}},
	}
	require.NoError(t, insert(queue))

	sender := GithubMergePR{
		ProjectID: projectID,
		Item:      itemID,
	}
	assert.NoError(t, sender.dequeueFromCommitQueue())

	queue, err := FindOneId(projectID)
	require.NoError(t, err)
	assert.Len(t, queue.Queue, 0)
}
