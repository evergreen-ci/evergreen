package route

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type StatsSuite struct {
	suite.Suite
}

func TestStatsSuite(t *testing.T) {
	suite.Run(t, new(StatsSuite))
}

func (s *StatsSuite) SetupSuite() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	proj := model.ProjectRef{
		Id: "project",
	}
	s.NoError(proj.Insert())
}

func (s *StatsSuite) TestParseStatsFilter() {
	values := url.Values{
		"requesters":  []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
		"after_date":  []string{"1998-07-12"},
		"before_date": []string{"2018-07-15"},
		"tests":       []string{"test1", "test2"},
		"tasks":       []string{"task1", "task2"},
		"variants":    []string{"v1,v2", "v3"},
	}
	handler := testStatsHandler{}

	err := handler.parseStatsFilter(values)
	s.Require().NoError(err)

	s.Equal([]string{
		evergreen.RepotrackerVersionRequester,
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.MergeTestRequester,
	}, handler.filter.Requesters)
	s.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
	s.Equal(time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
	s.Equal(values["tests"], handler.filter.Tests)
	s.Equal(values["tasks"], handler.filter.Tasks)
	s.Equal([]string{"v1", "v2", "v3"}, handler.filter.BuildVariants)
	s.Nil(handler.filter.Distros)
	s.Nil(handler.filter.StartAt)
	s.Equal(stats.GroupByDistro, handler.filter.GroupBy)  // default value
	s.Equal(stats.SortEarliestFirst, handler.filter.Sort) // default value
	s.Equal(statsAPIMaxLimit+1, handler.filter.Limit)     // default value
}

func (s *StatsSuite) TestRunTestHandler() {
	s.NoError(db.ClearCollections(stats.DailyTestStatsCollection, stats.DailyTaskStatsCollection))
	var err error
	handler := makeGetProjectTestStats("https://example.net/test", nil).(*testStatsHandler)
	s.Require().NoError(err)

	// 100 documents will be returned
	s.insertTestStats(handler, 100, 101)

	resp := handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Nil(resp.Pages())

	s.NoError(db.ClearCollections(stats.DailyTestStatsCollection, stats.DailyTaskStatsCollection))

	// 101 documents will be returned
	s.insertTestStats(handler, 101, 101)

	resp = handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Pages())
	docs, err := data.GetTestStats(handler.filter)
	s.NoError(err)
	s.Equal(docs[handler.filter.Limit-1].StartAtKey(), resp.Pages().Next.Key)
}

func (s *StatsSuite) TestReadTestStartAt() {
	handler := testStatsHandler{}
	startAt, err := handler.readStartAt("1998-07-12|variant1|task1|test1|distro1")
	s.Require().NoError(err)

	s.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), startAt.Date)
	s.Equal("variant1", startAt.BuildVariant)
	s.Equal("task1", startAt.Task)
	s.Equal("test1", startAt.Test)
	s.Equal("distro1", startAt.Distro)

	// Invalid format
	_, err = handler.readStartAt("1998-07-12|variant1|task1|test1")
	s.Require().Error(err)
}

func (s *StatsSuite) TestRunTaskHandler() {
	s.NoError(db.ClearCollections(stats.DailyTestStatsCollection, stats.DailyTaskStatsCollection))
	var err error
	handler := makeGetProjectTaskStats("https://example.net/task").(*taskStatsHandler)
	s.Require().NoError(err)

	// 100 documents will be returned
	s.insertTaskStats(handler, 100, 101)

	resp := handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Nil(resp.Pages())

	s.NoError(db.ClearCollections(stats.DailyTestStatsCollection, stats.DailyTaskStatsCollection))

	// 101 documents will be returned
	s.insertTaskStats(handler, 101, 101)

	resp = handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Pages())
	docs, err := data.GetTaskReliabilityScores(reliability.TaskReliabilityFilter{StatsFilter: handler.filter})
	s.NoError(err)
	s.Equal(docs[handler.filter.Limit-1].StartAtKey(), resp.Pages().Next.Key)
}

func (s *StatsSuite) insertTestStats(handler *testStatsHandler, numTests int, limit int) {
	day := time.Now()
	tests := []string{}
	for i := 0; i < numTests; i++ {
		testFile := fmt.Sprintf("%v%v", "test", i)
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
		s.Require().NoError(err)
	}
	handler.filter = stats.StatsFilter{
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
}

func (s *StatsSuite) insertTaskStats(handler *taskStatsHandler, numTests int, limit int) {
	day := time.Now()
	tasks := []string{}
	for i := 0; i < numTests; i++ {
		taskName := fmt.Sprintf("%v%v", "task", i)
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
		s.Require().NoError(err)
	}
	handler.filter = stats.StatsFilter{
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
}

func TestParsePrestoStatsFilter(t *testing.T) {
	now := time.Now()
	db, _, err := sqlmock.New()
	require.NoError(t, err)

	for _, test := range []struct {
		name           string
		vals           url.Values
		expectedFilter stats.PrestoTestStatsFilter
		hasErr         bool
	}{
		{
			name: "InvalidRequestType",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"requesters":  []string{"DNE"},
				"after_date":  []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{time.Now().Format(statsAPIDateFormat)},
			},
			hasErr: true,
		},
		{
			name: "InvalidAfterDate",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{"NOTADATE"},
				"before_date": []string{time.Now().Format(statsAPIDateFormat)},
			},
			hasErr: true,
		},
		{
			name: "InvalidBeforeDate",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{"NOTADATE"},
			},
			hasErr: true,
		},
		{
			name: "InvalidGroupBy",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{time.Now().Format(statsAPIDateFormat)},
				"group_by":    []string{statsAPITestGroupByTask},
			},
			hasErr: true,
		},
		{
			name: "InvalidGroupNumDays",
			vals: url.Values{
				"variants":       []string{"variant"},
				"tasks":          []string{"task"},
				"tests":          []string{"test_name"},
				"after_date":     []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date":    []string{time.Now().Format(statsAPIDateFormat)},
				"group_num_days": []string{"NOTANUM"},
			},
			hasErr: true,
		},
		{
			name: "GroupNumDaysNotEqualToOneOrNumDaysInDateRange",
			vals: url.Values{
				"variants":       []string{"variant"},
				"tasks":          []string{"task"},
				"tests":          []string{"test_name"},
				"after_date":     []string{time.Now().Add(-48 * time.Hour).Format(statsAPIDateFormat)},
				"before_date":    []string{time.Now().Format(statsAPIDateFormat)},
				"group_num_days": []string{"3"},
			},
			hasErr: true,
		},
		{
			name: "InvalidStartAt",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{time.Now().Format(statsAPIDateFormat)},
				"start_at":    []string{"NOTANUM"},
			},
			hasErr: true,
		},
		{
			name: "InvalidLimit",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{time.Now().Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{time.Now().Format(statsAPIDateFormat)},
				"limit":       []string{"NOTANUM"},
			},
			hasErr: true,
		},
		{
			name: "InvalidFilter",
			vals: url.Values{
				"tasks":       []string{"task"},
				"tests":       []string{"test_name"},
				"after_date":  []string{now.Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{now.Format(statsAPIDateFormat)},
			},
			hasErr: true,
		},
		{
			name: "DefaultValues",
			vals: url.Values{
				"variants":    []string{"variant"},
				"tasks":       []string{"task"},
				"tests":       []string{"test"},
				"after_date":  []string{now.Add(-24 * time.Hour).Format(statsAPIDateFormat)},
				"before_date": []string{now.Format(statsAPIDateFormat)},
			},
			expectedFilter: stats.PrestoTestStatsFilter{
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				AfterDate:  now.Add(-24 * time.Hour),
				BeforeDate: now,
			},
		},
		{
			name: "CustomValuesNoGroupDays",
			vals: url.Values{
				"variants": []string{"variant"},
				"tasks":    []string{"task"},
				"tests":    []string{"test"},
				"requesters": []string{
					statsAPIRequesterMainline,
					statsAPIRequesterTrigger,
					statsAPIRequesterGitTag,
					statsAPIRequesterAdhoc,
					statsAPIRequesterPatch,
				},
				"after_date":     []string{now.Add(-48 * time.Hour).Format(statsAPIDateFormat)},
				"before_date":    []string{now.Format(statsAPIDateFormat)},
				"start_at":       []string{"100"},
				"limit":          []string{"500"},
				"sort":           []string{statsAPISortLatest},
				"group_by":       []string{statsAPITestGroupByTest},
				"group_num_days": []string{"1"},
			},
			expectedFilter: stats.PrestoTestStatsFilter{
				Variant:  "variant",
				TaskName: "task",
				TestName: "test",
				Requesters: append(
					[]string{
						evergreen.RepotrackerVersionRequester,
						evergreen.TriggerRequester,
						evergreen.GitTagRequester,
						evergreen.AdHocRequester,
					},
					evergreen.PatchRequesters...,
				),
				AfterDate:   now.Add(-48 * time.Hour),
				BeforeDate:  now,
				Offset:      100,
				Limit:       501,
				SortDesc:    true,
				GroupByTest: true,
			},
		},
		{
			name: "CustomValuesGroupDays",
			vals: url.Values{
				"variants": []string{"variant"},
				"tasks":    []string{"task"},
				"tests":    []string{"test"},
				"requesters": []string{
					statsAPIRequesterMainline,
					statsAPIRequesterTrigger,
					statsAPIRequesterGitTag,
					statsAPIRequesterAdhoc,
					statsAPIRequesterPatch,
				},
				"after_date":     []string{now.Add(-48 * time.Hour).Format(statsAPIDateFormat)},
				"before_date":    []string{now.Format(statsAPIDateFormat)},
				"start_at":       []string{"100"},
				"limit":          []string{"500"},
				"sort":           []string{statsAPISortLatest},
				"group_by":       []string{statsAPITestGroupByTest},
				"group_num_days": []string{"2"},
			},
			expectedFilter: stats.PrestoTestStatsFilter{
				Variant:  "variant",
				TaskName: "task",
				TestName: "test",
				Requesters: append(
					[]string{
						evergreen.RepotrackerVersionRequester,
						evergreen.TriggerRequester,
						evergreen.GitTagRequester,
						evergreen.AdHocRequester,
					},
					evergreen.PatchRequesters...,
				),
				AfterDate:   now.Add(-48 * time.Hour),
				BeforeDate:  now,
				Offset:      100,
				Limit:       501,
				SortDesc:    true,
				GroupByTest: true,
				GroupDays:   true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := testStatsHandler{db: db}

			err := handler.parsePrestoStatsFilter("project", test.vals)
			if test.hasErr {
				assert.Error(t, err)
			} else {
				test.expectedFilter.Project = "project"
				test.expectedFilter.DB = db
				require.NoError(t, test.expectedFilter.Validate())

				require.NoError(t, err)
				assert.Equal(t, test.expectedFilter, *handler.prestoFilter)
			}
		})
	}
}

func TestPrestoTestStatsHandlerRun(t *testing.T) {
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
	handler := makeGetProjectTestStats("https://example.net/test", db).(*testStatsHandler)
	require.NoError(t, err)

	yesterday := utility.GetUTCDay(time.Now().Add(-24 * time.Hour))
	insertStats := func(query string, offset, limit int) []restModel.APITestStats {
		rows := sqlmock.NewRows([]string{"test_name", "task_name", "date", "num_pass", "num_fail", "average_duration"})
		expectedStats := make([]restModel.APITestStats, limit)
		for i := 0; i < len(expectedStats); i++ {
			expectedStats[i] = restModel.APITestStats{
				TestFile:        utility.ToStringPtr(fmt.Sprintf("test%d", i+offset)),
				TaskName:        utility.ToStringPtr("task"),
				BuildVariant:    utility.ToStringPtr("variant"),
				Distro:          utility.ToStringPtr(""),
				Date:            utility.ToStringPtr(yesterday.Format(statsAPIDateFormat)),
				NumPass:         rand.Intn(1000),
				NumFail:         rand.Intn(1000),
				AvgDurationPass: (time.Duration(rand.Int63n(100)) * time.Second).Seconds(),
			}
			rows.AddRow(expectedStats[i].TestFile, expectedStats[i].TaskName, yesterday, expectedStats[i].NumPass, expectedStats[i].NumFail, expectedStats[i].AvgDurationPass*1e9)
		}
		mock.ExpectQuery(query).WillReturnRows(rows)

		return expectedStats
	}

	t.Run("FirstPage", func(t *testing.T) {
		limit := 100
		handler.prestoFilter = &stats.PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  yesterday,
			BeforeDate: time.Now(),
			Limit:      limit + 1,
			DB:         db,
		}
		query, _, err := handler.prestoFilter.GenerateQuery()
		require.NoError(t, err)
		expectedStats := insertStats(query, 0, limit+1)

		resp := handler.Run(ctx)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.Status())
		assert.EqualValues(t, expectedStats[:limit], resp.Data())
		pages := resp.Pages()
		require.NotNil(t, pages)
		assert.Equal(t, "start_at", pages.Next.KeyQueryParam)
		assert.Equal(t, strconv.Itoa(limit), pages.Next.Key)
		assert.Equal(t, "limit", pages.Next.LimitQueryParam)
		assert.Equal(t, limit, pages.Next.Limit)
	})
	t.Run("SubsequentPage", func(t *testing.T) {
		limit := 100
		offset := 100
		handler.prestoFilter = &stats.PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  yesterday,
			BeforeDate: time.Now(),
			Offset:     offset,
			Limit:      limit + 1,
			DB:         db,
		}
		query, _, err := handler.prestoFilter.GenerateQuery()
		require.NoError(t, err)
		expectedStats := insertStats(query, offset, limit+1)

		resp := handler.Run(ctx)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.Status())
		assert.EqualValues(t, expectedStats[:limit], resp.Data())
		pages := resp.Pages()
		require.NotNil(t, pages)
		assert.Equal(t, "start_at", pages.Next.KeyQueryParam)
		assert.Equal(t, strconv.Itoa(limit+offset), pages.Next.Key)
		assert.Equal(t, "limit", pages.Next.LimitQueryParam)
		assert.Equal(t, limit, pages.Next.Limit)
	})
	t.Run("LastPage", func(t *testing.T) {
		handler.prestoFilter = &stats.PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  yesterday,
			BeforeDate: time.Now(),
			Limit:      101,
			Offset:     200,
			DB:         db,
		}
		query, _, err := handler.prestoFilter.GenerateQuery()
		require.NoError(t, err)
		expectedStats := insertStats(query, handler.prestoFilter.Offset, 50)

		resp := handler.Run(ctx)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.Status())
		assert.EqualValues(t, expectedStats, resp.Data())
		assert.Nil(t, resp.Pages())
	})
}
