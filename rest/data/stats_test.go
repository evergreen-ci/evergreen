package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockGetTestStats(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(stats.DailyTestStatsCollection, model.ProjectRefCollection))
	filter := &stats.StatsFilter{}
	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert())

	// Add stats
	assert.NoError(insertTestStats(filter, 102, 100))

	stats, err := GetTestStats(*filter)
	assert.NoError(err)
	assert.Len(stats, 100)

	assert.Equal(fmt.Sprintf("test_%v", 0), *stats[0].TestFile)
}

func TestMockGetTaskStats(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(stats.DailyTaskStatsCollection))
	filter := &stats.StatsFilter{}
	// Add stats
	assert.NoError(insertTaskStats(filter, 102, 100))

	stats, err := GetTaskStats(*filter)
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

	stats, err := GetTaskStats(stats.StatsFilter{
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

	stats, err := GetTestStats(stats.StatsFilter{
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

func TestGetPrestoTestStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	}()

	proj := model.ProjectRef{Id: "project"}
	require.NoError(t, proj.Insert())
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	for _, test := range []struct {
		name           string
		setup          func(*testing.T, stats.PrestoTestStatsFilter, []restModel.APITestStats)
		filter         stats.PrestoTestStatsFilter
		expectedResult []restModel.APITestStats
		hasErr         bool
	}{
		{
			name: "ProjectDNE",
			filter: stats.PrestoTestStatsFilter{
				Project:    "DNE",
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				AfterDate:  time.Now().Add(-48 * time.Hour),
				BeforeDate: time.Now().Add(24 * time.Hour),
				DB:         db,
			},
			hasErr: true,
		},
		{
			name: "PrestoError",
			setup: func(t *testing.T, filter stats.PrestoTestStatsFilter, _ []restModel.APITestStats) {
				query, _, err := filter.GenerateQuery()
				require.NoError(t, err)

				mock.ExpectQuery(query).WillReturnError(errors.New("query error"))
			},
			filter: stats.PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				AfterDate:  time.Now().Add(-48 * time.Hour),
				BeforeDate: time.Now().Add(24 * time.Hour),
				DB:         db,
			},
			hasErr: true,
		},
		{
			name: "SuccessfulPrestoCall",
			setup: func(t *testing.T, filter stats.PrestoTestStatsFilter, expected []restModel.APITestStats) {
				query, _, err := filter.GenerateQuery()
				require.NoError(t, err)

				rows := sqlmock.NewRows([]string{"test_name", "task_name", "date", "num_pass", "num_fail", "average_duration"})
				for _, row := range expected {
					date, err := time.Parse("2006-01-02", utility.FromStringPtr(row.Date))
					require.NoError(t, err)
					rows.AddRow(row.TestFile, row.TaskName, date, row.NumPass, row.NumFail, row.AvgDurationPass*1e9)
				}
				mock.ExpectQuery(query).WillReturnRows(rows)
			},
			filter: stats.PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				AfterDate:  time.Now().Add(-48 * time.Hour),
				BeforeDate: time.Now().Add(24 * time.Hour),
				DB:         db,
			},
			expectedResult: []restModel.APITestStats{
				{
					TestFile:        utility.ToStringPtr("test1"),
					TaskName:        utility.ToStringPtr("task1"),
					BuildVariant:    utility.ToStringPtr("variant"),
					Distro:          utility.ToStringPtr(""),
					Date:            utility.ToStringPtr("2022-01-01"),
					NumPass:         100,
					NumFail:         50,
					AvgDurationPass: (100 * time.Second).Seconds(),
				},
				{
					TestFile:        utility.ToStringPtr("test2"),
					TaskName:        utility.ToStringPtr("task2"),
					BuildVariant:    utility.ToStringPtr("variant"),
					Distro:          utility.ToStringPtr(""),
					Date:            utility.ToStringPtr("2022-01-02"),
					NumPass:         50,
					NumFail:         100,
					AvgDurationPass: (20 * time.Second).Seconds(),
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t, test.filter, test.expectedResult)
			}
			result, err := GetPrestoTestStats(ctx, test.filter)
			if test.hasErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedResult, result)
			}
		})
	}
}
