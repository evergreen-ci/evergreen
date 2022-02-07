package route

import (
	"bytes"
	"context"
	"fmt"
	"github.com/evergreen-ci/evergreen/db"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

// StatusSuite enables testing for version related routes.
type StatusSuite struct {
	sc   *data.DBConnector
	data data.DBStatusConnector
	h    *recentTasksGetHandler

	suite.Suite
}

func TestStatusSuite(t *testing.T) {
	suite.Run(t, new(StatusSuite))
}
func (s *StatusSuite) SetupSuite() {
	s.sc = &data.DBConnector{
		DBStatusConnector: data.DBStatusConnector{},
	}
}

func (s *StatusSuite) SetupTest() {
	s.NoError(db.Clear(task.Collection))
	tasks := []task.Task{
		{
			Id:         "task1",
			Status:     evergreen.TaskUndispatched,
			FinishTime: time.Now().Add(time.Second),
			DistroId:   "d2",
		},
		{
			Id:         "task2",
			Status:     evergreen.TaskStarted,
			FinishTime: time.Now().Add(time.Second),
			DistroId:   "d2",
		},
		{
			Id:         "task3",
			Status:     evergreen.TaskStarted,
			FinishTime: time.Now().Add(time.Second),
			DistroId:   "d3",
		},
		{
			Id:         "task4",
			Status:     evergreen.TaskStarted,
			FinishTime: time.Now().Add(time.Second),
			DistroId:   "d3",
		},
		{
			Id:     "task5",
			Status: evergreen.TaskFailed,
			Details: apimodels.TaskEndDetail{
				Type:     "system",
				TimedOut: true,
			},
			FinishTime: time.Now().Add(time.Second),
			DistroId:   "d3",
		},
	}
	for _, t := range tasks {
		t.Insert()
	}
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

func (s *StatusSuite) TestParseAndValidateByAgentVersion() {
	r, err := http.NewRequest("GET", "https://evergreen.mongodb.com/rest/v2/status/recent_tasks?by_agent_version=1", &bytes.Buffer{})
	s.Require().NoError(err)
	err = s.h.Parse(context.Background(), r)
	s.NoError(err)
	s.True(s.h.byAgentVersion)
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
	s.Equal(5, res.Total)
	s.Equal(3, res.Started)
	s.Equal(1, res.Inactive)
	s.Equal(1, res.SystemTimedOut)
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
		s.Equal(utility.ToStringPtr(fmt.Sprintf("task%d", i+1)), t.Id)
	}
}

func (s *StatusSuite) TestExecuteByDistro() {
	s.h.byDistro = true

	resp := s.h.Run(context.Background())
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp)
	res := *resp.Data().(*model.APIRecentTaskStatsList)

	s.Equal(utility.ToStringPtr("d3"), res["totals"][0].Name)
	s.Equal(3, res["totals"][0].Count)
	s.Equal(utility.ToStringPtr("d2"), res["totals"][1].Name)
	s.Equal(2, res["totals"][1].Count)

	s.Equal(utility.ToStringPtr("d3"), res[evergreen.TaskStarted][0].Name)
	s.Equal(2, res[evergreen.TaskStarted][0].Count)
	s.Equal(utility.ToStringPtr("d2"), res[evergreen.TaskStarted][1].Name)
	s.Equal(1, res[evergreen.TaskStarted][1].Count)

	s.Equal(utility.ToStringPtr("d2"), res[evergreen.TaskUndispatched][0].Name)
	s.Equal(1, res[evergreen.TaskUndispatched][0].Count)
}

func (s *StatusSuite) TaskTaskType() {
	s.h.minutes = 0
	s.h.verbose = true

	s.h.taskType = evergreen.TaskUnstarted
	resp := s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found := resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(utility.ToStringPtr("task1"), found.Id)

	s.h.taskType = evergreen.TaskStarted
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(utility.ToStringPtr("task2"), found.Id)

	s.h.taskType = evergreen.TaskSucceeded
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(utility.ToStringPtr("task3"), found.Id)
	found = resp.Data().([]interface{})[1].(*model.APITask)
	s.Equal(utility.ToStringPtr("task4"), found.Id)

	s.h.taskType = evergreen.TaskSystemTimedOut
	resp = s.h.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	found = resp.Data().([]interface{})[0].(*model.APITask)
	s.Equal(utility.ToStringPtr("task5"), found.Id)
}
