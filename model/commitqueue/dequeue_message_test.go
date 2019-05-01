package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
)

func TestDequeueItem(t *testing.T) {
	assert := assert.New(t)
	dequeue := DequeueItem{
		ProjectID: "mci",
		Item:      "abcdef",
		Status:    evergreen.PatchFailed,
	}
	c := NewDequeueItemMessage(level.Info, dequeue)
	assert.NotNil(c)
	assert.True(c.Loggable())

	raw, ok := c.Raw().(*DequeueItem)
	assert.True(ok)

	assert.Equal(dequeue, *raw)

	assert.Equal("commit queue 'mci' item 'abcdef'", c.String())
}

func TestDequeueItemLoggable(t *testing.T) {
	assert := assert.New(t)

	missingProject := DequeueItem{Item: "abcdef", Status: evergreen.PatchFailed}
	c := NewDequeueItemMessage(level.Info, missingProject)
	assert.False(c.Loggable())

	missingItem := DequeueItem{ProjectID: "mci", Status: evergreen.PatchFailed}
	c = NewDequeueItemMessage(level.Info, missingItem)
	assert.False(c.Loggable())

	missingStatus := DequeueItem{ProjectID: "mci", Item: "abcdef"}
	c = NewDequeueItemMessage(level.Info, missingStatus)
	assert.False(c.Loggable())

	validMessage := DequeueItem{
		ProjectID: "mci",
		Item:      "abcdef",
		Status:    evergreen.PatchFailed,
	}
	c = NewDequeueItemMessage(level.Info, validMessage)
	assert.True(c.Loggable())
}
