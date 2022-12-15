package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dayInHours = 24 * time.Hour

func TestMockGetTaskReliability(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, taskstats.DailyTaskStatsCollection))

	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert())
	filter := reliability.TaskReliabilityFilter{
		StatsFilter: taskstats.StatsFilter{
			Limit:        100,
			Project:      "project",
			Requesters:   []string{"requester"},
			Tasks:        []string{"task1"},
			GroupBy:      "distro",
			GroupNumDays: 1,
			Sort:         taskstats.SortEarliestFirst,
			BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
			AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
		},
	}
	scores, err := GetTaskReliabilityScores(filter)
	assert.NoError(err)
	assert.Len(scores, 0)

	// Add stats
	day := time.Now()
	tasks := []string{}
	for i := 0; i < 102; i++ {
		taskName := fmt.Sprintf("%v%v", "task_", i)
		tasks = append(tasks, taskName)
		err = db.Insert(taskstats.DailyTaskStatsCollection, mgobson.M{
			"_id": taskstats.DbTaskStatsId{
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
		StatsFilter: taskstats.StatsFilter{
			Limit:        100,
			Project:      "project",
			Requesters:   []string{"requester"},
			Tasks:        tasks,
			GroupBy:      "distro",
			GroupNumDays: 1,
			Sort:         taskstats.SortEarliestFirst,
			BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
			AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
		},
	}

	scores, err = GetTaskReliabilityScores(filter)
	assert.NoError(err)
	assert.Len(scores, 100)

	var date *string
	for i, doc := range scores {
		assert.Contains(*doc.TaskName, "task_")
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
		assert.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection, model.ProjectRefCollection))
	}()
	assert.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection, model.ProjectRefCollection))

	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert())
	stat := taskstats.DbTaskStats{
		Id: taskstats.DbTaskStatsId{
			Project:   "projectID",
			TaskName:  "t0",
			Date:      time.Date(2022, 02, 15, 0, 0, 0, 0, time.UTC),
			Requester: evergreen.RepotrackerVersionRequester,
		},
	}
	assert.NoError(t, db.Insert(taskstats.DailyTaskStatsCollection, stat))
	projectRef := model.ProjectRef{
		Id:         "projectID",
		Identifier: "projectName",
	}
	assert.NoError(t, projectRef.Insert())

	filter := reliability.TaskReliabilityFilter{}
	filter.Project = "projectName"
	filter.GroupNumDays = 1
	filter.Requesters = []string{evergreen.RepotrackerVersionRequester}
	filter.Sort = taskstats.SortLatestFirst
	filter.GroupBy = taskstats.GroupByTask
	filter.AfterDate = time.Time{}
	filter.BeforeDate = time.Date(2022, 02, 16, 0, 0, 0, 0, time.UTC)
	filter.Limit = 1
	filter.Tasks = []string{"t0"}
	scores, err := GetTaskReliabilityScores(filter)

	assert.NoError(t, err)
	require.Len(t, scores, 1)
}
