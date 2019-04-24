package amboy

import (
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

func TestQueueStats(t *testing.T) {
	assert := assert.New(t)
	stat := &QueueStats{}

	assert.Implements((*message.Composer)(nil), stat)

	assert.True(stat.IsComplete())

	assert.Contains(stat.String(), "running='0'")
	assert.Contains(stat.String(), "pending='0'")

	stat.Total++
	assert.False(stat.IsComplete())
	assert.Contains(stat.String(), "total='1'")

	stat.Completed++
	assert.True(stat.IsComplete())
	assert.Contains(stat.String(), "completed='1'")

	stat.Blocked++
	assert.True(stat.IsComplete())
	assert.Contains(stat.String(), "blocked='1'")

	stat.Total++
	assert.True(stat.IsComplete())
	assert.Contains(stat.String(), "total='2'")
}
