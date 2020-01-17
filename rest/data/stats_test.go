package data

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/stretchr/testify/assert"
)

func TestMockGetTestStats(t *testing.T) {
	assert := assert.New(t)

	mock := MockStatsConnector{}
	filter := stats.StatsFilter{Limit: 100}

	stats, err := mock.GetTestStats(filter)
	assert.NoError(err)
	assert.Len(stats, 0)

	// Add stats
	mock.SetTestStats("test_", 102)

	stats, err = mock.GetTestStats(filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	var date *string
	for i, doc := range stats {
		assert.Equal(fmt.Sprintf("test_%v", i), *doc.TestFile)
		assert.Equal("task", *doc.TaskName)
		assert.Equal("variant", *doc.BuildVariant)
		assert.Equal("distro", *doc.Distro)
		if i == 0 {
			date = doc.Date
		} else {
			assert.Equal(date, doc.Date)
		}
	}
}

func TestMockGetTaskStats(t *testing.T) {
	assert := assert.New(t)

	mock := MockStatsConnector{}
	filter := stats.StatsFilter{Limit: 100}

	stats, err := mock.GetTaskStats(filter)
	assert.NoError(err)
	assert.Len(stats, 0)

	// Add stats
	mock.SetTaskStats("task_", 102)

	stats, err = mock.GetTaskStats(filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	var date *string
	for i, doc := range stats {
		assert.Equal(fmt.Sprintf("task_%v", i), *doc.TaskName)
		assert.Equal("variant", *doc.BuildVariant)
		assert.Equal("distro", *doc.Distro)
		if i == 0 {
			date = doc.Date
		} else {
			assert.Equal(date, doc.Date)
		}
	}
}
