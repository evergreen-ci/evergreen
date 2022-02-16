package data

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/stretchr/testify/assert"
)

func TestMockGetTestStats(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(stats.DailyTestStatsCollection))
	mock := StatsConnector{}
	filter := &stats.StatsFilter{}

	// Add stats
	assert.NoError(insertTestStats(filter, 102, 100))

	stats, err := mock.GetTestStats(*filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	assert.Equal(fmt.Sprintf("test_%v", 0), *stats[0].TestFile)
}

func TestMockGetTaskStats(t *testing.T) {
	assert := assert.New(t)
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
