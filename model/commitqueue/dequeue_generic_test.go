package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDequeueItemSend(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))
	projectID := "evergreen"
	itemID := "abcdef"
	queue := &CommitQueue{
		ProjectID: projectID,
		Queue:     []CommitQueueItem{{Issue: itemID}},
	}
	require.NoError(t, insert(queue))

	dequeueMessage := DequeueItem{
		ProjectID: projectID,
		Item:      itemID,
	}

	assert.NoError(t, dequeueMessage.Send())
	queue, err := FindOneId(projectID)
	require.NoError(t, err)
	assert.Empty(t, queue.Queue)

	events, err := event.FindUnprocessedEvents(0)
	require.NoError(t, err)
	require.Len(t, events, 1)
}
