package data

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"

	"github.com/stretchr/testify/assert"
)

func TestMockGetTaskReliability(t *testing.T) {
	assert := assert.New(t)

	mock := MockTaskReliabilityConnector{}
	filter := reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 100,
		},
	}

	stats, err := mock.GetTaskReliabilityScores(filter)
	assert.NoError(err)
	assert.Len(stats, 0)

	// Add stats
	mock.SetTaskReliabilityScores("task_", 102)

	stats, err = mock.GetTaskReliabilityScores(filter)
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
