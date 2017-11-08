package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

// StatusSuite enables testing for version related routes.
type StatusSuite struct {
	sc   *data.MockConnector
	data data.MockStatusConnector
	h    *recentTasksGetHandler

	suite.Suite
}

func TestStatusSuite(t *testing.T) {
	suite.Run(t, new(StatusSuite))
}
func (s *StatusSuite) SetupSuite() {
	s.data = data.MockStatusConnector{
		CachedTasks: []task.Task{
			{
				Id:     "task1",
				Status: evergreen.TaskUndispatched,
			},
			{
				Id:     "task2",
				Status: evergreen.TaskStarted,
			},
			{
				Id:     "task3",
				Status: evergreen.TaskStarted,
			},
			{
				Id:     "task4",
				Status: evergreen.TaskStarted,
			},
			{
				Id:     "task5",
				Status: evergreen.TaskFailed,
				Details: apimodels.TaskEndDetail{
					Type:     "system",
					TimedOut: true,
				},
			},
		},
		CachedResults: &task.ResultCounts{
			Total:              1,
			Inactive:           2,
			Unstarted:          3,
			Started:            4,
			Succeeded:          5,
			Failed:             6,
			SystemFailed:       7,
			SystemUnresponsive: 8,
			SystemTimedOut:     9,
			TestTimedOut:       10,
		},
	}
	s.sc = &data.MockConnector{
		MockStatusConnector: s.data,
	}
}

func (s *StatusSuite) SetupTest() {
	s.h = &recentTasksGetHandler{}
}

func (s *StatusSuite) TestParseAndValidateDefault() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.NoError(err)
	s.Equal(30, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateMinutes() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=5", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.NoError(err)
	s.Equal(5, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateMinutesAndVerbose() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=5&verbose=true", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.NoError(err)
	s.Equal(5, s.h.minutes)
	s.Equal(true, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateVerbose() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?verbose=true", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.NoError(err)
	s.Equal(30, s.h.minutes)
	s.Equal(true, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateMaxMinutes() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=1500", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.Error(err)
	s.Equal(0, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateNegativeMinutesAreParsedPositive() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=-10", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.ParseAndValidate(context.Background(), r)
	s.Error(err)
	s.Equal(0, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestExecuteDefault() {
	s.h.minutes = 0
	s.h.verbose = false

	resp, err := s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 1)
	res := resp.Result[0].(*model.APITaskStats)
	s.Equal(1, res.Total)
	s.Equal(2, res.Inactive)
	s.Equal(3, res.Unstarted)
	s.Equal(4, res.Started)
	s.Equal(5, res.Succeeded)
	s.Equal(6, res.Failed)
	s.Equal(7, res.SystemFailed)
	s.Equal(8, res.SystemUnresponsive)
	s.Equal(9, res.SystemTimedOut)
	s.Equal(10, res.TestTimedOut)
}

func (s *StatusSuite) TestExecuteVerbose() {
	s.h.minutes = 0
	s.h.verbose = true

	resp, err := s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 5)
	for i, result := range resp.Result {
		t := result.(*model.APITask)
		s.Equal(model.APIString(fmt.Sprintf("task%d", i+1)), t.Id)
	}
}

func (s *StatusSuite) TaskTaskType() {
	s.h.minutes = 0
	s.h.verbose = true

	s.h.taskType = evergreen.TaskUnstarted
	resp, err := s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 1)
	found := resp.Result[0].(*model.APITask)
	s.Equal(model.APIString("task1"), found.Id)

	s.h.taskType = evergreen.TaskStarted
	resp, err = s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 1)
	found = resp.Result[0].(*model.APITask)
	s.Equal(model.APIString("task2"), found.Id)

	s.h.taskType = evergreen.TaskSucceeded
	resp, err = s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 2)
	found = resp.Result[0].(*model.APITask)
	s.Equal(model.APIString("task3"), found.Id)
	found = resp.Result[1].(*model.APITask)
	s.Equal(model.APIString("task4"), found.Id)

	s.h.taskType = evergreen.TaskSystemTimedOut
	resp, err = s.h.Execute(context.Background(), s.sc)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Result, 1)
	found = resp.Result[0].(*model.APITask)
	s.Equal(model.APIString("task5"), found.Id)
}
