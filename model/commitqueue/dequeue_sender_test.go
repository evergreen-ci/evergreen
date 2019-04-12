package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

func TestCommitQueueDequeueLogger(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(Collection))
	q := &CommitQueue{
		ProjectID: "mci",
		Queue: []CommitQueueItem{
			CommitQueueItem{
				Issue: "1",
			},
			CommitQueueItem{
				Issue: "2",
			},
		},
	}
	assert.NoError(InsertQueue(q))
	assert.NoError(q.SetProcessing(true))

	msg := NewDequeueItemMessage(level.Notice, DequeueItem{
		ProjectID: "mci",
		Item:      "1",
	})
	sender, err := NewCommitQueueDequeueLogger("dq sender", send.LevelInfo{
		Default:   level.Notice,
		Threshold: level.Notice,
	})
	assert.NoError(err)

	dequeueSender, ok := sender.(*commitQueueDequeueLogger)
	assert.True(ok)
	assert.NoError(dequeueSender.doSend(msg))

	q, err = FindOneId("mci")
	assert.NoError(err)
	assert.False(q.Processing)
	assert.Equal("2", q.Next().Issue)
}
