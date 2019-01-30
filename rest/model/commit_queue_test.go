package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/stretchr/testify/assert"
)

func TestCommitQueueBuildFromService(t *testing.T) {
	assert := assert.New(t)
	cq := commitqueue.CommitQueue{
		ProjectID:    "mci",
		Queue:        []string{"1", "2", "3"},
		MergeAction:  "squash",
		StatusAction: "github",
	}

	cqAPI := APICommitQueue{}
	assert.NoError(cqAPI.BuildFromService(cq))
	assert.Equal(cq.ProjectID, FromAPIString(cqAPI.ProjectID))
	for i, item := range cq.Queue {
		assert.Equal(item, FromAPIString(cqAPI.Queue[i]))
	}
}
