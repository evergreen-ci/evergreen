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
		CachedResultCountList: &model.APIRecentTaskStatsList{
			Total: model.APIStatList{
				{Name: model.ToAPIString("d1"), Count: 3},
				{Name: model.ToAPIString("d2"), Count: 2},
			},
			Inactive: model.APIStatList{
				{Name: model.ToAPIString("d1"), Count: 2},
				{Name: model.ToAPIString("d2"), Count: 1},
			},
			Succeeded: model.APIStatList{
				{Name: model.ToAPIString("d1"), Count: 1},
				{Name: model.ToAPIString("d2"), Count: 1},
			},
		},
	}
	s.sc = &data.MockConnector{
		MockStatusConnector: s.data,
	}
}

func (s *StatusSuite) SetupTest() {
	s.h = &recentTasksGetHandler{sc: s.sc}
}

func (s *StatusSuite) TestParseAndValidateDefault() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.Equal(30, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateMinutes() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=5", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.Equal(5, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateByDistro() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?by_distro=true", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.True(s.h.byDistro)
}

func (s *StatusSuite) TestParseAndValidateByProject() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?by_project=1", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.True(s.h.byProject)
}

func (s *StatusSuite) TestParseAndValidateByDistroAndProject() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?by_distro=true&by_project=1", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.Error(err)
}

func (s *StatusSuite) TestParseAndValidateMinutesAndVerbose() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=5&verbose=true", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.Equal(5, s.h.minutes)
	s.Equal(true, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateVerbose() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?verbose=true", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.Equal(30, s.h.minutes)
	s.Equal(true, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateMaxMinutes() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=1500", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.Error(err)
	s.Equal(0, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestParseAndValidateNegativeMinutesAreParsedPositive() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?minutes=-10", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.Error(err)
	s.Equal(0, s.h.minutes)
	s.Equal(false, s.h.verbose)
}

func (s *StatusSuite) TestExecuteDefault() {
	s.h.minutes = 0
	s.h.verbose = false

	resp := s.h.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp)
	res := resp.Data().(*model.APIRecentTaskStats)
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

	resp := s.h.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp)
	s.Len(resp.Data().([]model.Model), 5)
	for i, result := range resp.Data().([]model.Model) {
		t := result.(*model.APITask)
		s.Equal(model.ToAPIString(fmt.Sprintf("task%d", i+1)), t.Id)
	}
}

func (s *StatusSuite) TestExecuteByDistro() {
	s.h.byDistro = true

	resp := s.h.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp)
	res := resp.Data().(*model.APIRecentTaskStatsList)

	s.Equal(model.ToAPIString("d1"), res.Total[0].Name)
	s.Equal(3, res.Total[0].Count)
	s.Equal(model.ToAPIString("d2"), res.Total[1].Name)
	s.Equal(2, res.Total[1].Count)

	s.Equal(model.ToAPIString("d1"), res.Inactive[0].Name)
	s.Equal(2, res.Inactive[0].Count)
	s.Equal(model.ToAPIString("d2"), res.Inactive[1].Name)
	s.Equal(1, res.Inactive[1].Count)

	s.Equal(model.ToAPIString("d1"), res.Succeeded[0].Name)
	s.Equal(1, res.Succeeded[0].Count)
	s.Equal(model.ToAPIString("d2"), res.Succeeded[1].Name)
	s.Equal(1, res.Succeeded[1].Count)
}

func (s *StatusSuite) TaskTaskType() {
	s.h.minutes = 0
	s.h.verbose = true

	s.h.taskType = evergreen.TaskUnstarted
	resp := s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found := resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(model.ToAPIString("task1"), found.Id)

	s.h.taskType = evergreen.TaskStarted
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(model.ToAPIString("task2"), found.Id)

	s.h.taskType = evergreen.TaskSucceeded
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(model.ToAPIString("task3"), found.Id)
	found = resp.Data().([]interface{})[1].(*model.APITask)
	s.Equal(model.ToAPIString("task4"), found.Id)

	s.h.taskType = evergreen.TaskSystemTimedOut
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(model.ToAPIString("task5"), found.Id)
}
