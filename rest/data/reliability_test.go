package data

import (
	"context"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dayInHours = 24 * time.Hour

func TestMockGetTaskReliability(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	mock := TaskReliabilityConnector{}
	filter := reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit:        100,
			Project:      "project",
			Requesters:   []string{"requester"},
			Tasks:        []string{"task1"},
			GroupBy:      "distro",
			GroupNumDays: 1,
			Sort:         stats.SortEarliestFirst,
			BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
			AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
		},
	}
	scores, err := mock.GetTaskReliabilityScores(filter)
	assert.NoError(err)
	assert.Len(scores, 0)

	// Add stats
	//mock.SetTaskReliabilityScores("task_", 102)
	day := time.Now()
	tasks := []string{}
	for i := 0; i < 102; i++ {
		taskName := fmt.Sprintf("%v%v", "task_", i)
		tasks = append(tasks, taskName)
		err = db.Insert(stats.DailyTaskStatsCollection, mgobson.M{
			"_id": stats.DbTaskStatsId{
				Project:      "project",
				Requester:    "requester",
				TaskName:     taskName,
				BuildVariant: "variant",
				Distro:       "distro",
				Date:         utility.GetUTCDay(day),
			},
		})
		require.NoError(t, err)
	}
	filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit:        100,
			Project:      "project",
			Requesters:   []string{"requester"},
			Tasks:        tasks,
			GroupBy:      "distro",
			GroupNumDays: 1,
			Sort:         stats.SortEarliestFirst,
			BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
			AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
		},
	}

	scores, err = mock.GetTaskReliabilityScores(filter)
	assert.NoError(err)
	assert.Len(scores, 100)

	var date *string
	for i, doc := range scores {
		assert.Contains("task_", *doc.TaskName)
		assert.Equal("variant", *doc.BuildVariant)
		assert.Equal("distro", *doc.Distro)
		if i == 0 {
			date = doc.Date
		} else {
			assert.Equal(date, doc.Date)
		}
	}
}
