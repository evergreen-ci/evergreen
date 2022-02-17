package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetTaskReliability(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(stats.DailyTaskStatsCollection, model.ProjectRefCollection))
	}()
	assert.NoError(t, db.ClearCollections(stats.DailyTaskStatsCollection, model.ProjectRefCollection))

	stat := stats.DbTaskStats{
		Id: stats.DbTaskStatsId{
			Project:   "projectID",
			TaskName:  "t0",
			Date:      time.Date(2022, 02, 15, 0, 0, 0, 0, time.UTC),
			Requester: evergreen.RepotrackerVersionRequester,
		},
	}
	assert.NoError(t, db.Insert(stats.DailyTaskStatsCollection, stat))
	projectRef := model.ProjectRef{
		Id:         "projectID",
		Identifier: "projectName",
	}
	assert.NoError(t, projectRef.Insert())

	sc := TaskReliabilityConnector{}
	filter := reliability.TaskReliabilityFilter{}
	filter.Project = "projectName"
	filter.GroupNumDays = 1
	filter.Requesters = []string{evergreen.RepotrackerVersionRequester}
	filter.Sort = stats.SortLatestFirst
	filter.GroupBy = stats.GroupByTask
	filter.AfterDate = time.Time{}
	filter.BeforeDate = time.Date(2022, 02, 16, 0, 0, 0, 0, time.UTC)
	filter.Limit = 1
	filter.Tasks = []string{"t0"}
	scores, err := sc.GetTaskReliabilityScores(filter)

	assert.NoError(t, err)
	require.Len(t, scores, 1)
}
