package amboy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueueStats(t *testing.T) {
	assert := assert.New(t)
	stat := QueueStats{}

	assert.True(stat.isComplete())

	assert.Contains(stat.String(), "running='0'")
	assert.Contains(stat.String(), "pending='0'")

	stat.Total++
	assert.False(stat.isComplete())
	assert.Contains(stat.String(), "total='1'")

	stat.Completed++
	assert.True(stat.isComplete())
	assert.Contains(stat.String(), "completed='1'")

	stat.Blocked++
	assert.True(stat.isComplete())
	assert.Contains(stat.String(), "blocked='1'")

	stat.Total++
	assert.True(stat.isComplete())
	assert.Contains(stat.String(), "total='2'")
}
