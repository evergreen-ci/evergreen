package route

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type TaskStatsSuite struct {
	suite.Suite
}

func TestTaskStatsSuite(t *testing.T) {
	suite.Run(t, new(TaskStatsSuite))
}

func (s *TaskStatsSuite) SetupSuite() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	proj := model.ProjectRef{
		Id: "project",
	}
	s.NoError(proj.Insert(s.T().Context()))
}

func (s *TaskStatsSuite) TestParseStatsFilter() {
	values := url.Values{
		"requesters":  []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
		"after_date":  []string{"1998-07-12"},
		"before_date": []string{"2018-07-15"},
		"tasks":       []string{"task1", "task2"},
		"variants":    []string{"v1,v2", "v3"},
	}
	handler := taskStatsHandler{}

	err := handler.parseStatsFilter(values)
	s.Require().NoError(err)

	s.Equal([]string{
		evergreen.RepotrackerVersionRequester,
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.GithubMergeRequester,
	}, handler.filter.Requesters)
	s.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
	s.Equal(time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
	s.Equal(values["tasks"], handler.filter.Tasks)
	s.Equal([]string{"v1", "v2", "v3"}, handler.filter.BuildVariants)
	s.Nil(handler.filter.Distros)
	s.Nil(handler.filter.StartAt)
	s.Equal(taskstats.GroupByDistro, handler.filter.GroupBy)  // default value
	s.Equal(taskstats.SortEarliestFirst, handler.filter.Sort) // default value
	s.Equal(statsAPIMaxLimit+1, handler.filter.Limit)         // default value
}

func (s *TaskStatsSuite) TestRunTaskHandler() {
	s.Require().NoError(db.ClearCollections(taskstats.DailyTaskStatsCollection))

	handler := makeGetProjectTaskStats().(*taskStatsHandler)
	handler.url = "https://example.net/task"

	// 100 documents will be returned
	s.insertTaskStats(handler, 100, 101)

	resp := handler.Run(context.Background())
	s.Require().NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Nil(resp.Pages())

	s.Require().NoError(db.ClearCollections(taskstats.DailyTaskStatsCollection))

	// 101 documents will be returned
	s.insertTaskStats(handler, 101, 101)

	resp = handler.Run(context.Background())
	s.Require().NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Pages())

	docs, err := data.GetTaskReliabilityScores(s.T().Context(), reliability.TaskReliabilityFilter{StatsFilter: handler.filter})
	s.Require().NoError(err)
	s.Equal(docs[handler.filter.Limit-1].StartAtKey(), resp.Pages().Next.Key)
}

func (s *TaskStatsSuite) insertTaskStats(handler *taskStatsHandler, numTests int, limit int) {
	day := time.Now()
	tasks := []string{}
	for i := 0; i < numTests; i++ {
		taskName := fmt.Sprintf("%v%v", "task", i)
		tasks = append(tasks, taskName)
		err := db.Insert(s.T().Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
			"_id": taskstats.DBTaskStatsID{
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
	handler.filter = taskstats.StatsFilter{
		Limit:        limit,
		Project:      "project",
		Requesters:   []string{"requester"},
		Tasks:        tasks,
		GroupBy:      "distro",
		GroupNumDays: 1,
		Sort:         taskstats.SortEarliestFirst,
		BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
		AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
	}
}
