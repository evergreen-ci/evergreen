package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/stretchr/testify/assert"
)

func TestCommitQueueBuildFromService(t *testing.T) {
	assert := assert.New(t)
	cq := commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "1",
				Modules: []commitqueue.Module{
					commitqueue.Module{
						Module: "test_module",
						Issue:  "2",
					},
				},
			},
			commitqueue.CommitQueueItem{
				Issue: "2",
			},
			commitqueue.CommitQueueItem{
				Issue: "3",
			},
		},
	}

	cqAPI := APICommitQueue{}
	assert.NoError(cqAPI.BuildFromService(cq))
	assert.Equal(cq.ProjectID, FromAPIString(cqAPI.ProjectID))
	assert.Equal(len(cqAPI.Queue), len(cq.Queue))
	for i := range cq.Queue {
		assert.Equal(cq.Queue[i].Issue, FromAPIString(cqAPI.Queue[i].Issue))
	}
	assert.Equal(cq.Queue[0].Modules[0].Module, FromAPIString(cqAPI.Queue[0].Modules[0].Module))
	assert.Equal(cq.Queue[0].Modules[0].Issue, FromAPIString(cqAPI.Queue[0].Modules[0].Issue))
}
