package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockGetTestStats(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	assert.NoError(db.ClearCollections(stats.DailyTestStatsCollection, model.ProjectRefCollection))
	mock := StatsConnector{}
	filter := &stats.StatsFilter{}
	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert())

	// Add stats
	assert.NoError(insertTestStats(filter, 102, 100))

	stats, err := mock.GetTestStats(*filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	assert.Equal(fmt.Sprintf("test_%v", 0), *stats[0].TestFile)
}

func TestMockGetTaskStats(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	assert.NoError(db.Clear(stats.DailyTaskStatsCollection))
	mock := StatsConnector{}
	filter := &stats.StatsFilter{}
	// Add stats
	assert.NoError(insertTaskStats(filter, 102, 100))

	stats, err := mock.GetTaskStats(*filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	assert.Equal(fmt.Sprintf("task_%v", 0), *stats[0].TaskName)

}

func insertTestStats(filter *stats.StatsFilter, numTests int, limit int) error {
	day := time.Now()
	tests := []string{}
	for i := 0; i < numTests; i++ {
		testFile := fmt.Sprintf("%v%v", "test_", i)
		tests = append(tests, testFile)
		err := db.Insert(stats.DailyTestStatsCollection, mgobson.M{
			"_id": stats.DbTestStatsId{
				Project:      "project",
				Requester:    "requester",
				TestFile:     testFile,
				TaskName:     "task",
				BuildVariant: "variant",
				Distro:       "distro",
				Date:         utility.GetUTCDay(day),
			},
		})
		if err != nil {
			return err
		}
	}
	*filter = stats.StatsFilter{
		Limit:        limit,
		Project:      "project",
		Requesters:   []string{"requester"},
		Tasks:        []string{"task"},
		GroupBy:      "distro",
		GroupNumDays: 1,
		Tests:        tests,
		Sort:         stats.SortEarliestFirst,
		BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
		AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
	}
	return nil
}

func insertTaskStats(filter *stats.StatsFilter, numTests int, limit int) error {
	day := time.Now()
	tasks := []string{}
	for i := 0; i < numTests; i++ {
		taskName := fmt.Sprintf("%v%v", "task_", i)
		tasks = append(tasks, taskName)
		err := db.Insert(stats.DailyTaskStatsCollection, mgobson.M{
			"_id": stats.DbTestStatsId{
				Project:      "project",
				Requester:    "requester",
				TaskName:     taskName,
				BuildVariant: "variant",
				Distro:       "distro",
				Date:         utility.GetUTCDay(day),
			},
		})
		if err != nil {
			return err
		}
	}
	*filter = stats.StatsFilter{
		Limit:        limit,
		Project:      "project",
		Requesters:   []string{"requester"},
		Tasks:        tasks,
		GroupBy:      "distro",
		GroupNumDays: 1,
		Sort:         stats.SortEarliestFirst,
		BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
		AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
	}
	return nil
}

func TestGetTaskStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
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
