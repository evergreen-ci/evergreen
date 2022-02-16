package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetTaskStats(t *testing.T) {
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

	sc := StatsConnector{}
	stats, err := sc.GetTaskStats(stats.StatsFilter{
		Project:      "projectName",
		GroupNumDays: 1,
		Requesters:   []string{evergreen.RepotrackerVersionRequester},
		Sort:         stats.SortLatestFirst,
		GroupBy:      stats.GroupByTask,
		AfterDate:    time.Time{},
		BeforeDate:   time.Date(2022, 02, 16, 0, 0, 0, 0, time.UTC),
		Limit:        1,
		Tasks:        []string{"t0"},
	})
	assert.NoError(t, err)
	require.Len(t, stats, 1)
}

func TestGetTestStats(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(stats.DailyTestStatsCollection, model.ProjectRefCollection))
	}()
	assert.NoError(t, db.ClearCollections(stats.DailyTestStatsCollection, model.ProjectRefCollection))

	stat := stats.DbTestStats{
		Id: stats.DbTestStatsId{
			Project:   "projectID",
			TaskName:  "t0",
			Date:      time.Date(2022, 02, 15, 0, 0, 0, 0, time.UTC),
			Requester: evergreen.RepotrackerVersionRequester,
		},
		LastID: bson.NewObjectId(),
	}
	assert.NoError(t, db.Insert(stats.DailyTestStatsCollection, stat))
	projectRef := model.ProjectRef{
		Id:         "projectID",
		Identifier: "projectName",
	}
	assert.NoError(t, projectRef.Insert())

	sc := StatsConnector{}
	stats, err := sc.GetTestStats(stats.StatsFilter{
		Project:      "projectName",
		GroupNumDays: 1,
		Requesters:   []string{evergreen.RepotrackerVersionRequester},
		Sort:         stats.SortLatestFirst,
		GroupBy:      stats.GroupByTask,
		AfterDate:    time.Time{},
		BeforeDate:   time.Date(2022, 02, 16, 0, 0, 0, 0, time.UTC),
		Limit:        1,
		Tasks:        []string{"t0"},
	})
	assert.NoError(t, err)
	require.Len(t, stats, 1)
}
