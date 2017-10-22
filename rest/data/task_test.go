package data

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// This is the testConfig used in every data test
// We need to move it somewhere more appropriate!
var (
	testConfig = testutil.TestConfig()
)

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch task by id route

type TaskConnectorFetchByIdSuite struct {
	ctx Connector

	suite.Suite
}

func TestTaskConnectorFetchByIdSuite(t *testing.T) {
	s := new(TaskConnectorFetchByIdSuite)
	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskConnectorFetchByIdSuite")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	assert.NoError(t, db.Clear(task.Collection))

	for i := 0; i < 10; i++ {
		testTask := &task.Task{
			Id:      fmt.Sprintf("task_%d", i),
			BuildId: fmt.Sprintf("build_%d", i),
		}
		assert.NoError(t, testTask.Insert())
	}
	suite.Run(t, s)
}

func (s *TaskConnectorFetchByIdSuite) TestFindById() {
	for i := 0; i < 10; i++ {
		found, err := s.ctx.FindTaskById(fmt.Sprintf("task_%d", i))
		s.Nil(err)
		s.Equal(found.BuildId, fmt.Sprintf("build_%d", i))
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByIdFail() {
	found, err := s.ctx.FindTaskById("fake_task")
	s.NotNil(err)
	s.Nil(found)

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch task by build route

type TaskConnectorFetchByBuildSuite struct {
	ctx       Connector
	taskIds   [][]string
	numTasks  int
	numBuilds int

	suite.Suite
}

func TestTaskConnectorFetchByBuildSuite(t *testing.T) {
	s := new(TaskConnectorFetchByBuildSuite)
	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskConnectorFetchByBuildSuite")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	assert.NoError(t, db.Clear(task.Collection))

	s.taskIds = make([][]string, 2)
	s.numTasks = 16
	s.numBuilds = 2

	for bix := 0; bix < s.numBuilds; bix++ {
		tids := make([]string, s.numTasks)
		for tix := range tids {
			tids[tix] = fmt.Sprintf("task_%d_build_%d", tix, bix)
		}
		sort.StringSlice(tids).Sort()
		s.taskIds[bix] = tids
	}

	for bix := 0; bix < s.numBuilds; bix++ {
		for tix, tid := range s.taskIds[bix] {
			status := "pass"
			if (tix % 2) == 0 {
				status = "fail"
			}
			testTask := &task.Task{
				Id:      tid,
				BuildId: fmt.Sprintf("build_%d", bix),
				Status:  status,
			}
			assert.NoError(t, testTask.Insert())
		}
	}

	suite.Run(t, s)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindByBuild() {
	for bix := 0; bix < s.numBuilds; bix++ {
		foundTasks, err := s.ctx.FindTasksByBuildId(fmt.Sprintf("build_%d", bix),
			"", "", 0, 1)
		s.Nil(err)
		s.Equal(s.numTasks, len(foundTasks))
		for tix, t := range foundTasks {
			s.Equal(s.taskIds[bix][tix], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindByBuildFail() {
	for _, status := range []string{"pass", "fail"} {
		for bix := 0; bix < s.numBuilds; bix++ {
			foundTasks, err := s.ctx.FindTasksByBuildId(fmt.Sprintf("build_%d", bix),
				"", status, 0, 1)
			s.Nil(err)
			s.Equal(s.numTasks/2, len(foundTasks))
			for _, t := range foundTasks {
				s.Equal(status, t.Status)
			}
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindByBuildAndStatus() {
	buildId := "build_1"
	tids := s.taskIds[1]
	for _, sort := range []int{1, -1} {
		for i := 0; i < s.numTasks; i++ {
			foundTasks, err := s.ctx.FindTasksByBuildId(buildId, tids[i],
				"", 0, sort)
			s.Nil(err)

			startAt := 0
			if sort < 0 {
				startAt = len(tids) - 1
			}

			s.Equal((s.numTasks-startAt)-i*sort, len(foundTasks))
			for ix, t := range foundTasks {
				index := ix
				if sort > 0 {
					index += i
				}
				s.Equal(tids[index], t.Id)
			}
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddle() {
	buildId := "build_0"
	limit := 2
	tids := s.taskIds[0]
	for i := 0; i < s.numTasks/limit; i++ {
		index := i * limit
		taskName := tids[index]
		foundTasks, err := s.ctx.FindTasksByBuildId(buildId, taskName,
			"", limit, 1)
		s.Nil(err)
		s.Equal(limit, len(foundTasks))
		for ix, t := range foundTasks {
			s.Equal(tids[ix+index], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleTaskFail() {
	foundTests, err := s.ctx.FindTasksByBuildId("build_0", "fake_task", "", 0, 1)
	s.NotNil(err)
	s.Equal(0, len(foundTests))

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleBuildFail() {
	foundTests, err := s.ctx.FindTasksByBuildId("fake_build", "", "", 0, 1)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}
func (s *TaskConnectorFetchByBuildSuite) TestFindEmptyTaskId() {
	buildId := "build_0"
	foundTasks, err := s.ctx.FindTasksByBuildId(buildId, "", "", 1, 1)
	s.Nil(err)
	s.Equal(1, len(foundTasks))
	task1 := foundTasks[0]
	s.Equal(s.taskIds[0][0], task1.Id)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch task by project and commit route

type TaskConnectorFetchByProjectAndCommitSuite struct {
	ctx         Connector
	numCommits  int
	numProjects int
	numTasks    int
	taskIds     [][][]string

	suite.Suite
}

func TestTaskConnectorFetchByProjectAndCommitSuite(t *testing.T) {
	s := new(TaskConnectorFetchByProjectAndCommitSuite)
	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestTaskConnectorFetchByProjectAndCommitSuite")
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	assert.NoError(t, db.Clear(task.Collection))

	s.numCommits = 2
	s.numProjects = 2
	s.numTasks = 16

	s.taskIds = make([][][]string, s.numProjects)

	for pix := 0; pix < s.numProjects; pix++ {
		s.taskIds[pix] = make([][]string, s.numCommits)
		for cix := 0; cix < s.numCommits; cix++ {
			tids := make([]string, s.numTasks)
			for tix := range tids {
				tids[tix] = fmt.Sprintf("task_%d_project%d_commit%d", tix, pix, cix)
			}
			sort.StringSlice(tids).Sort()
			s.taskIds[pix][cix] = tids
		}
	}

	for cix := 0; cix < s.numCommits; cix++ {
		for pix := 0; pix < s.numProjects; pix++ {
			for tix, tid := range s.taskIds[pix][cix] {
				status := "pass"
				if (tix % 2) == 0 {
					status = "fail"
				}
				testTask := &task.Task{
					Id:       tid,
					Revision: fmt.Sprintf("commit_%d", cix),
					Project:  fmt.Sprintf("project_%d", pix),
					Status:   status,
				}
				assert.NoError(t, testTask.Insert())
			}
		}
	}

	suite.Run(t, s)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommit() {
	for pix := 0; pix < s.numProjects; pix++ {
		for cix := 0; cix < s.numCommits; cix++ {
			foundTasks, err := s.ctx.FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
				fmt.Sprintf("commit_%d", cix), "", "", 0, 1)
			s.NoError(err)
			s.Equal(s.numTasks, len(foundTasks))
			for tix, t := range foundTasks {
				s.Equal(s.taskIds[pix][cix][tix], t.Id)
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("fake_project", "commit_0", "", "", 0, 1)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByCommitFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("project_0", "fake_commit", "", "", 0, 1)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommitAndStatus() {
	for _, status := range []string{"pass", "fail"} {
		for pix := 0; pix < s.numProjects; pix++ {
			for cix := 0; cix < s.numCommits; cix++ {
				foundTasks, err := s.ctx.FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
					fmt.Sprintf("commit_%d", cix), "", status, 0, 1)
				s.Nil(err)
				s.Equal(s.numTasks/2, len(foundTasks))
				for _, t := range foundTasks {
					s.Equal(status, t.Status)
				}
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindFromMiddle() {
	commitId := "commit_1"
	projectId := "project_1"
	tids := s.taskIds[1][1]
	for _, sort := range []int{1, -1} {
		for i := 0; i < s.numTasks; i++ {
			foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId,
				tids[i], "", 0, sort)
			s.NoError(err)

			startAt := 0
			if sort < 0 {
				startAt = len(tids) - 1
			}

			s.Equal((s.numTasks-startAt)-i*sort, len(foundTasks))
			for ix, t := range foundTasks {
				index := ix
				if sort > 0 {
					index += i
				}
				s.Equal(tids[index], t.Id)
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindFromMiddleFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("project_0", "commit_0", "fake_task", "", 0, 1)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(&rest.APIError{}, err)
	apiErr, ok := err.(*rest.APIError)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindWithLimit() {
	commitId := "commit_0"
	projectId := "project_0"
	limit := 2
	tids := s.taskIds[0][0]
	for i := 0; i < s.numTasks/limit; i++ {
		index := i * limit
		taskName := tids[index]
		foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId,
			taskName, "", limit, 1)
		s.NoError(err)
		s.Equal(limit, len(foundTasks))
		for ix, t := range foundTasks {
			s.Equal(tids[ix+index], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindEmptyProjectAndCommit() {
	projectId := "project_0"
	commitId := "commit_0"
	foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId, "", "", 1, 1)
	s.NoError(err)
	s.Equal(1, len(foundTasks))
	task1 := foundTasks[0]
	s.Equal(s.taskIds[0][0][0], task1.Id)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for abort task route

type TaskConnectorAbortTaskSuite struct {
	ctx Connector

	suite.Suite
}

func TestMockTaskConnectorAbortTaskSuite(t *testing.T) {
	s := new(TaskConnectorAbortTaskSuite)
	s.ctx = &MockConnector{MockTaskConnector: MockTaskConnector{
		CachedTasks:   []task.Task{{Id: "task1"}},
		CachedAborted: make(map[string]string),
	}}
	suite.Run(t, s)
}

func (s *TaskConnectorAbortTaskSuite) TestAbort() {
	err := s.ctx.AbortTask("task1", "user1")
	s.NoError(err)
	s.Equal("user1", s.ctx.(*MockConnector).MockTaskConnector.CachedAborted["task1"])
}

func (s *TaskConnectorAbortTaskSuite) TestAbortFail() {
	s.ctx.(*MockConnector).MockTaskConnector.FailOnAbort = true
	err := s.ctx.AbortTask("task1", "user1")
	s.Error(err)
}
