package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

func TestCommitQueueDequeueLogger(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(Collection, event.AllLogCollection))
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
	assert.Equal("2", q.Queue[0].Issue)
	eventLog, err := event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
	assert.NoError(err)
	assert.Len(eventLog, 1)

	// dequeue a non-existent item
	msg = NewDequeueItemMessage(level.Notice, DequeueItem{
		ProjectID: "mci",
		Item:      "1",
	})
	assert.Error(dequeueSender.doSend(msg))
	// no additional events are logged
	eventLog, err = event.FindUnprocessedEvents(evergreen.DefaultEventProcessingLimit)
	assert.NoError(err)
	assert.Len(eventLog, 1)
}
