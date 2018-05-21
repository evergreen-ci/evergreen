package route

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
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
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "user1"})

	rm := getTaskAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*taskAbortHandler).taskId = "task1"
	res, err := rm.Methods[0].Execute(ctx, s.sc)

	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["task1"])
	s.Equal("", s.data.CachedAborted["task2"])
	t, ok := (res.Result[0]).(*model.APITask)
	s.True(ok)
	s.Equal(model.ToAPIString("task1"), t.Id)

	res, err = rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal("user1", s.data.CachedAborted["task1"])
	s.Equal("", s.data.CachedAborted["task2"])
	t, ok = (res.Result[0]).(*model.APITask)
	s.True(ok)
	s.Equal(model.ToAPIString("task1"), t.Id)
}

func (s *TaskAbortSuite) TestAbortFail() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, evergreen.RequestUser, &user.DBUser{Id: "user1"})

	rm := getTaskAbortManager("", 2)
	(rm.Methods[0].RequestHandler).(*taskAbortHandler).taskId = "task1"
	s.sc.MockTaskConnector.FailOnAbort = true
	_, err := rm.Methods[0].Execute(ctx, s.sc)

	s.Error(err)
}

func TestFetchArtifacts(t *testing.T) {
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert.NoError(db.ClearCollections(task.Collection, artifact.Collection))
	task1 := task.Task{
		Id:        "task1",
		Execution: 0,
	}
	assert.NoError(task1.Insert())
	entry := artifact.Entry{
		TaskId:          task1.Id,
		TaskDisplayName: "task",
		BuildId:         "b1",
		Execution:       0,
		Files: []artifact.File{
			{
				Name: "file1",
				Link: "l1",
			},
			{
				Name: "file2",
				Link: "l2",
			},
		},
	}
	assert.NoError(entry.Upsert())

	taskGet := taskGetHandler{taskID: task1.Id}
	resp, err := taskGet.Execute(context.Background(), &data.DBConnector{})
	assert.NoError(err)
	apiTask := resp.Result[0].(*model.APITask)
	assert.Len(apiTask.Artifacts, 2)
}
