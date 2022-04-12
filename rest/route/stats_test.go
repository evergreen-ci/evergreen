package route

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type StatsSuite struct {
	suite.Suite
}

func TestStatsSuite(t *testing.T) {
	// kim: TODO: remove
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()
	// env := testutil.NewEnvironment(ctx, t)
	// evergreen.SetEnvironment(env)
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
	handler := makeGetProjectTestStats("https://example.net/test").(*testStatsHandler)
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
