package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

////////////////////////////////////////////////////////////////////////
//
// Tests for abort task route

type TaskAbortSuite struct {
	sc   *data.MockConnector
	data data.MockTaskConnector

	suite.Suite
}

func TestTaskAbortSuite(t *testing.T) {
	suite.Run(t, new(TaskAbortSuite))
}

func (s *TaskAbortSuite) SetupSuite() {
	s.data = data.MockTaskConnector{
		CachedTasks: []task.Task{
			{Id: "task1"},
			{Id: "task2"},
		},
		CachedAborted: make(map[string]string),
	}
	s.sc = &data.MockConnector{
		MockTaskConnector: s.data,
	}
}

func (s *TaskAbortSuite) TestAbort() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getTaskAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*taskAbortHandler).taskId = "task1"
	res, err := rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["task1"])
	s.Equal("", s.data.CachedAborted["task2"])
	t, ok := (res.Result[0]).(*model.APITask)
	s.True(ok)
	s.Equal(model.APIString("task1"), t.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["task1"])
	s.Equal("", s.data.CachedAborted["task2"])
	t, ok = (res.Result[0]).(*model.APITask)
	s.True(ok)
	s.Equal(model.APIString("task1"), t.Id)
}

func (s *TaskAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestUser, &user.DBUser{Id: "user1"})

	rm := getTaskAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*taskAbortHandler).taskId = "task1"
	s.sc.MockTaskConnector.FailOnAbort = true
	_, err := rm.Methods[0].Execute(ctx, s.sc)

	s.Error(err)
}
