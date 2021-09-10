package data

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen/model"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
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

	s := &TaskConnectorFetchByIdSuite{
		ctx: &DBConnector{},
	}

	suite.Run(t, s)
}

func (s *TaskConnectorFetchByIdSuite) SetupTest() {
	s.Require().NoError(db.Clear(task.Collection))
	for i := 0; i < 10; i++ {
		testTask := &task.Task{
			Id:      fmt.Sprintf("task_%d", i),
			BuildId: fmt.Sprintf("build_%d", i),
		}
		s.NoError(testTask.Insert())
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindById() {
	for i := 0; i < 10; i++ {
		found, err := s.ctx.FindTaskById(fmt.Sprintf("task_%d", i))
		s.Nil(err)
		s.Equal(found.BuildId, fmt.Sprintf("build_%d", i))
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByIdAndExecution() {
	s.Require().NoError(db.ClearCollections(task.Collection, task.OldCollection))
	testTask1 := &task.Task{
		Id:        "task_1",
		Execution: 0,
		BuildId:   "build_1",
	}
	s.NoError(testTask1.Insert())
	for i := 0; i < 10; i++ {
		s.NoError(testTask1.Archive())
		testTask1.Execution += 1
	}
	for i := 0; i < 10; i++ {
		task, err := s.ctx.FindTaskByIdAndExecution("task_1", i)
		s.NoError(err)
		s.Equal(task.Id, fmt.Sprintf("task_1_%d", i))
		s.Equal(task.Execution, i)
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByVersion() {
	s.Require().NoError(db.ClearCollections(task.Collection, task.OldCollection, annotations.Collection))
	task_known_2 := &task.Task{
		Id:        "task_known",
		Execution: 2,
		Version:   "version_known",
		Status:    evergreen.TaskSucceeded,
	}
	task_not_known := &task.Task{
		Id:        "task_not_known",
		Execution: 0,
		Version:   "version_not_known",
		Status:    evergreen.TaskFailed,
	}
	task_no_annotation := &task.Task{
		Id:        "task_no_annotation",
		Execution: 0,
		Version:   "version_no_annotation",
		Status:    evergreen.TaskFailed,
	}
	task_with_empty_issues := &task.Task{
		Id:        "task_with_empty_issues",
		Execution: 0,
		Version:   "version_with_empty_issues",
		Status:    evergreen.TaskFailed,
	}
	s.NoError(task_known_2.Insert())
	s.NoError(task_not_known.Insert())
	s.NoError(task_no_annotation.Insert())
	s.NoError(task_with_empty_issues.Insert())

	issue := annotations.IssueLink{URL: "https://issuelink.com", IssueKey: "EVG-1234", Source: &annotations.Source{Author: "chaya.malik"}}

	a_execution_0 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue}}
	a_execution_1 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 1, SuspectedIssues: []annotations.IssueLink{issue}}
	a_execution_2 := annotations.TaskAnnotation{TaskId: "task_known", TaskExecution: 2, Issues: []annotations.IssueLink{issue}}

	a_with__suspected_issue := annotations.TaskAnnotation{TaskId: "task_not_known", TaskExecution: 0, SuspectedIssues: []annotations.IssueLink{issue}}
	a_with_empty_issues := annotations.TaskAnnotation{TaskId: "task_not_known", TaskExecution: 0, Issues: []annotations.IssueLink{}, SuspectedIssues: []annotations.IssueLink{issue}}

	s.NoError(a_execution_0.Upsert())
	s.NoError(a_execution_1.Upsert())
	s.NoError(a_execution_2.Upsert())
	s.NoError(a_with__suspected_issue.Upsert())
	s.NoError(a_with_empty_issues.Upsert())

	opts := TaskFilterOptions{}

	task, _, err := s.ctx.FindTasksByVersion("version_known", opts)
	s.NoError(err)
	// ignore annotation for successful task
	s.Equal(evergreen.TaskSucceeded, task[0].DisplayStatus)

	// test with empty issues list
	task, _, err = s.ctx.FindTasksByVersion("version_not_known", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, task[0].DisplayStatus)

	// test with no annotation document
	task, _, err = s.ctx.FindTasksByVersion("version_no_annotation", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, task[0].DisplayStatus)

	// test with empty issues
	task, _, err = s.ctx.FindTasksByVersion("version_with_empty_issues", opts)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, task[0].DisplayStatus)

}

func (s *TaskConnectorFetchByIdSuite) TestFindOldTasksByIDWithDisplayTasks() {
	s.Require().NoError(db.ClearCollections(task.Collection, task.OldCollection))
	testTask1 := &task.Task{
		Id:        "task_1",
		Execution: 0,
		BuildId:   "build_1",
	}
	s.NoError(testTask1.Insert())
	testTask2 := &task.Task{
		Id:          "task_2",
		Execution:   0,
		BuildId:     "build_1",
		DisplayOnly: true,
	}
	s.NoError(testTask2.Insert())
	for i := 0; i < 10; i++ {
		s.NoError(testTask1.Archive())
		testTask1.Execution += 1
		s.NoError(testTask2.Archive())
		testTask2.Execution += 1
	}

	tasks, err := s.ctx.FindOldTasksByIDWithDisplayTasks("task_1")
	s.NoError(err)
	s.Len(tasks, 10)
	for i := range tasks {
		s.Equal(i, tasks[i].Execution)
	}

	tasks, err = s.ctx.FindOldTasksByIDWithDisplayTasks("task_2")
	s.NoError(err)
	s.Len(tasks, 10)
	for i := range tasks {
		s.Equal(i, tasks[i].Execution)
	}
}

func (s *TaskConnectorFetchByIdSuite) TestFindByIdFail() {
	found, err := s.ctx.FindTaskById("fake_task")
	s.NotNil(err)
	s.Nil(found)

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
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
				var index int
				if sort > 0 {
					index = i + ix
				} else {
					index = (len(foundTasks) - 1) - ix
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

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleBuildFail() {
	foundTests, err := s.ctx.FindTasksByBuildId("fake_build", "", "", 0, 1)
	s.NoError(err)
	s.Equal(0, len(foundTests))
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

	assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection))

	s.numCommits = 2
	s.numProjects = 2
	s.numTasks = 16

	s.taskIds = make([][][]string, s.numProjects)

	for pix := 0; pix < s.numProjects; pix++ {
		s.taskIds[pix] = make([][]string, s.numCommits)
		pRef := model.ProjectRef{
			Id: fmt.Sprintf("project_%d", pix),
		}
		assert.NoError(t, pRef.Insert())
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
				fmt.Sprintf("commit_%d", cix), "", "", 0)
			s.NoError(err)
			s.Equal(s.numTasks, len(foundTasks))
			for tix, t := range foundTasks {
				s.Equal(s.taskIds[pix][cix][tix], t.Id)
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("fake_project", "commit_0", "", "", 0)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByCommitFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("project_0", "fake_commit", "", "", 0)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommitAndStatus() {
	for _, status := range []string{"pass", "fail"} {
		for pix := 0; pix < s.numProjects; pix++ {
			for cix := 0; cix < s.numCommits; cix++ {
				foundTasks, err := s.ctx.FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
					fmt.Sprintf("commit_%d", cix), "", status, 0)
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
	for i := 0; i < s.numTasks; i++ {
		foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId, tids[i], "", 0)
		s.NoError(err)

		startAt := 0

		s.Equal((s.numTasks-startAt)-i, len(foundTasks))
		for ix, t := range foundTasks {
			index := ix + i
			s.Equal(tids[index], t.Id)
		}
	}

}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindFromMiddleFail() {
	foundTests, err := s.ctx.FindTasksByProjectAndCommit("project_0", "commit_0", "fake_task", "", 0)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
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
		foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId, taskName, "", limit)
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
	foundTasks, err := s.ctx.FindTasksByProjectAndCommit(projectId, commitId, "", "", 1)
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

func TestCheckTaskSecret(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))

	ctx := &DBConnector{}

	task := task.Task{
		Id:     "task1",
		Secret: "abcdef",
	}
	assert.NoError(task.Insert())

	r := &http.Request{
		Header: http.Header{
			evergreen.TaskHeader: []string{"task1"},
		},
	}
	code, err := ctx.CheckTaskSecret("task1", r)
	assert.Error(err)
	assert.Equal(http.StatusUnauthorized, code)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	code, err = ctx.CheckTaskSecret("task1", r)
	assert.NoError(err)
	assert.Equal(http.StatusOK, code)
}
