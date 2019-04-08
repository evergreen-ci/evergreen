package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TaskConnectorSuite struct {
	ctx       Connector
	starttime time.Time
	suite.Suite
}

func TestTaskConnectorSuite(t *testing.T) {
	assert := assert.New(t)

	// Set up
	s := new(TaskConnectorSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskConnectorSuite")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	// Tear down
	assert.NoError(db.Clear(task.Collection))

	s.starttime = time.Now()

	testTask1 := &task.Task{
		Id:           "task1",
		DisplayName:  "displname1",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(1)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}

	testTask2 := &task.Task{
		Id:           "task2",
		DisplayName:  "displname2",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(1)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}
	assert.NoError(testTask1.Insert())
	assert.NoError(testTask2.Insert())

	suite.Run(t, s)
}

func TestMockTaskConnectorSuite(t *testing.T) {
	s := new(TaskConnectorSuite)

	s.starttime = time.Now()

	testTask1 := task.Task{
		Id:           "task1",
		DisplayName:  "displname1",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(1)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}

	testTask2 := task.Task{
		Id:           "task2",
		DisplayName:  "displname2",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(1)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}

	s.ctx = &MockConnector{
		MockTaskConnector: MockTaskConnector{
			CachedTasks: []task.Task{testTask1, testTask2},
		},
	}
	suite.Run(t, s)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectAscendingExact() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "", s.starttime,
		endtime, 2, 1)
	s.NoError(err)
	s.Equal(2, len(tasks))
	s.Equal("task1", tasks[0].Id)
	s.Equal("task2", tasks[1].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectDescendingExact() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "z", s.starttime,
		endtime, 2, -1)
	s.NoError(err)
	s.Equal(2, len(tasks))
	s.Equal("task2", tasks[0].Id)
	s.Equal("task1", tasks[1].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectAscendingTooMany() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "", s.starttime,
		endtime, 4, 1)
	s.NoError(err)
	s.Equal(2, len(tasks))
	s.Equal("task1", tasks[0].Id)
	s.Equal("task2", tasks[1].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectDescendingTooMany() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "z", s.starttime,
		endtime, 4, -1)
	s.NoError(err)
	s.Equal(2, len(tasks))
	s.Equal("task2", tasks[0].Id)
	s.Equal("task1", tasks[1].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectAscendingTooFew() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "", s.starttime,
		endtime, 1, 1)
	s.NoError(err)
	s.Equal(1, len(tasks))
	s.Equal("task1", tasks[0].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectDescendingTooFew() {
	endtime := s.starttime.Add(time.Duration(2))
	tasks, err := s.ctx.FindCostTaskByProject("project", "z", s.starttime,
		endtime, 1, -1)
	s.NoError(err)
	s.Equal(1, len(tasks))
	s.Equal("task2", tasks[0].Id)
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectNoTaskInTimeRange() {
	addtime, _ := time.ParseDuration("1m")
	starttime := s.starttime.Add(addtime)
	endtime := starttime.Add(addtime)

	tasks, err := s.ctx.FindCostTaskByProject("project", "", starttime,
		endtime, 1, 1)
	s.NoError(err)
	s.Equal(0, len(tasks))
}

func (s *TaskConnectorSuite) TestFindCostTaskByProjectNoProject() {
	endtime := s.starttime.Add(time.Duration(2))

	tasks, err := s.ctx.FindCostTaskByProject("fake_project", "z", s.starttime,
		endtime, 1, -1)
	s.NoError(err)
	s.Equal(0, len(tasks))
}
