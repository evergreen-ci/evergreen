package data

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
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
// Tests for fetch task by build route

type TaskConnectorFetchByBuildSuite struct {
	taskIds   [][]string
	numTasks  int
	numBuilds int

	suite.Suite
}

func TestTaskConnectorFetchByBuildSuite(t *testing.T) {
	s := new(TaskConnectorFetchByBuildSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
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
		foundTasks, err := FindTasksByBuildId(fmt.Sprintf("build_%d", bix),
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
			foundTasks, err := FindTasksByBuildId(fmt.Sprintf("build_%d", bix),
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
			foundTasks, err := FindTasksByBuildId(buildId, tids[i],
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
		foundTasks, err := FindTasksByBuildId(buildId, taskName,
			"", limit, 1)
		s.Nil(err)
		s.Equal(limit, len(foundTasks))
		for ix, t := range foundTasks {
			s.Equal(tids[ix+index], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleTaskFail() {
	foundTests, err := FindTasksByBuildId("build_0", "fake_task", "", 0, 1)
	s.NotNil(err)
	s.Equal(0, len(foundTests))

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleBuildFail() {
	foundTests, err := FindTasksByBuildId("fake_build", "", "", 0, 1)
	s.NoError(err)
	s.Equal(0, len(foundTests))
}
func (s *TaskConnectorFetchByBuildSuite) TestFindEmptyTaskId() {
	buildId := "build_0"
	foundTasks, err := FindTasksByBuildId(buildId, "", "", 1, 1)
	s.Nil(err)
	s.Equal(1, len(foundTasks))
	task1 := foundTasks[0]
	s.Equal(s.taskIds[0][0], task1.Id)
}

////////////////////////////////////////////////////////////////////////
//
// Tests for fetch task by project and commit route

type TaskConnectorFetchByProjectAndCommitSuite struct {
	numCommits  int
	numProjects int
	numTasks    int
	taskIds     [][][]string

	suite.Suite
}

func TestTaskConnectorFetchByProjectAndCommitSuite(t *testing.T) {
	s := new(TaskConnectorFetchByProjectAndCommitSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
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
			foundTasks, err := FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
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
	foundTests, err := FindTasksByProjectAndCommit("fake_project", "commit_0", "", "", 0)
	s.Error(err)
	s.Equal(0, len(foundTests))

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByCommitFail() {
	foundTests, err := FindTasksByProjectAndCommit("project_0", "fake_commit", "", "", 0)
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
				foundTasks, err := FindTasksByProjectAndCommit(fmt.Sprintf("project_%d", pix),
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
		foundTasks, err := FindTasksByProjectAndCommit(projectId, commitId, tids[i], "", 0)
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
	foundTests, err := FindTasksByProjectAndCommit("project_0", "commit_0", "fake_task", "", 0)
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
		foundTasks, err := FindTasksByProjectAndCommit(projectId, commitId, taskName, "", limit)
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
	foundTasks, err := FindTasksByProjectAndCommit(projectId, commitId, "", "", 1)
	s.NoError(err)
	s.Equal(1, len(foundTasks))
	task1 := foundTasks[0]
	s.Equal(s.taskIds[0][0][0], task1.Id)
}

func TestCheckTaskSecret(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	assert.NoError(db.ClearCollections(task.Collection))

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
	code, err := CheckTaskSecret("task1", r)
	assert.Error(err)
	assert.Equal(http.StatusUnauthorized, code)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	code, err = CheckTaskSecret("task1", r)
	assert.NoError(err)
	assert.Equal(http.StatusOK, code)
}
