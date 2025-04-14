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
	ctx       context.Context

	suite.Suite
}

func TestTaskConnectorFetchByBuildSuite(t *testing.T) {
	s := new(TaskConnectorFetchByBuildSuite)
	assert.NoError(t, db.Clear(task.Collection))

	s.taskIds = make([][]string, 2)
	s.numTasks = 16
	s.numBuilds = 2
	var cancel context.CancelFunc
	s.ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

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
			assert.NoError(t, testTask.Insert(t.Context()))
		}
	}

	suite.Run(t, s)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindByBuild() {
	for bix := 0; bix < s.numBuilds; bix++ {
		foundTasks, err := FindTasksByBuildId(s.ctx, fmt.Sprintf("build_%d", bix),
			"", "", 0, 1)
		s.NoError(err)
		s.Len(foundTasks, s.numTasks)
		for tix, t := range foundTasks {
			s.Equal(s.taskIds[bix][tix], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindByBuildFail() {
	for _, status := range []string{"pass", "fail"} {
		for bix := 0; bix < s.numBuilds; bix++ {
			foundTasks, err := FindTasksByBuildId(s.ctx, fmt.Sprintf("build_%d", bix),
				"", status, 0, 1)
			s.NoError(err)
			s.Len(foundTasks, s.numTasks/2)
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
			foundTasks, err := FindTasksByBuildId(s.ctx, buildId, tids[i],
				"", 0, sort)
			s.NoError(err)

			startAt := 0
			if sort < 0 {
				startAt = len(tids) - 1
			}

			s.Len(foundTasks, (s.numTasks-startAt)-i*sort)
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
		foundTasks, err := FindTasksByBuildId(s.ctx, buildId, taskName,
			"", limit, 1)
		s.NoError(err)
		s.Len(foundTasks, limit)
		for ix, t := range foundTasks {
			s.Equal(tids[ix+index], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleTaskFail() {
	foundTests, err := FindTasksByBuildId(s.ctx, "build_0", "fake_task", "", 0, 1)
	s.Error(err)
	s.Empty(foundTests)

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByBuildSuite) TestFindFromMiddleBuildFail() {
	foundTests, err := FindTasksByBuildId(s.ctx, "fake_build", "", "", 0, 1)
	s.NoError(err)
	s.Empty(foundTests)
}
func (s *TaskConnectorFetchByBuildSuite) TestFindEmptyTaskId() {
	buildId := "build_0"
	foundTasks, err := FindTasksByBuildId(s.ctx, buildId, "", "", 1, 1)
	s.NoError(err)
	s.Len(foundTasks, 1)
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
	ctx         context.Context

	suite.Suite
}

func TestTaskConnectorFetchByProjectAndCommitSuite(t *testing.T) {
	s := new(TaskConnectorFetchByProjectAndCommitSuite)
	assert.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection))

	s.numCommits = 2
	s.numProjects = 2
	s.numTasks = 16
	var cancel context.CancelFunc
	s.ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	s.taskIds = make([][][]string, s.numProjects)

	for pix := 0; pix < s.numProjects; pix++ {
		s.taskIds[pix] = make([][]string, s.numCommits)
		pRef := model.ProjectRef{
			Id: fmt.Sprintf("project_%d", pix),
		}
		assert.NoError(t, pRef.Insert(t.Context()))
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
				variant := "bv1"
				if (tix % 2) == 0 {
					status = "fail"
					variant = "bv2"
				}
				testTask := &task.Task{
					Id:           tid,
					Revision:     fmt.Sprintf("commit_%d", cix),
					Project:      fmt.Sprintf("project_%d", pix),
					Status:       status,
					BuildVariant: variant,
					DisplayName:  fmt.Sprintf("task_%d", tix),
					Requester:    evergreen.RepotrackerVersionRequester,
				}
				assert.NoError(t, testTask.Insert(t.Context()))
			}
		}
	}

	suite.Run(t, s)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommit() {
	for pix := 0; pix < s.numProjects; pix++ {
		for cix := 0; cix < s.numCommits; cix++ {
			opts := task.GetTasksByProjectAndCommitOptions{
				Project:        fmt.Sprintf("project_%d", pix),
				CommitHash:     fmt.Sprintf("commit_%d", cix),
				StartingTaskId: "",
				Status:         "",
				TaskName:       "",
				VariantName:    "",
				Limit:          0,
			}
			foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
			s.NoError(err)
			s.Len(foundTasks, s.numTasks)
			for tix, t := range foundTasks {
				s.Equal(s.taskIds[pix][cix][tix], t.Id)
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestRegexFindByProjectAndCommit() {
	opts := task.GetTasksByProjectAndCommitOptions{
		Project:        "project_0",
		CommitHash:     "commit_0",
		StartingTaskId: "",
		Status:         "",
		TaskName:       "",
		VariantName:    "",
		VariantRegex:   "^bv",
		Limit:          0,
	}
	foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
	s.NoError(err)
	s.Len(foundTasks, 16)

	opts.VariantRegex = "1$"
	foundTasks, err = FindTasksByProjectAndCommit(s.ctx, opts)
	s.NoError(err)
	s.Len(foundTasks, 8)

	opts.VariantRegex = "2$"
	foundTasks, err = FindTasksByProjectAndCommit(s.ctx, opts)
	s.NoError(err)
	s.Len(foundTasks, 8)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectFail() {
	opts := task.GetTasksByProjectAndCommitOptions{
		Project:        "fake_project",
		CommitHash:     "commit_0",
		StartingTaskId: "",
		Status:         "",
		TaskName:       "",
		VariantName:    "",
		Limit:          0,
	}
	foundTests, err := FindTasksByProjectAndCommit(s.ctx, opts)
	s.Error(err)
	s.Empty(foundTests)

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByCommitFail() {
	opts := task.GetTasksByProjectAndCommitOptions{
		Project:        "project_0",
		CommitHash:     "fake_commit",
		StartingTaskId: "",
		Status:         "",
		TaskName:       "",
		VariantName:    "",
		Limit:          0,
	}
	foundTests, err := FindTasksByProjectAndCommit(s.ctx, opts)
	s.Error(err)
	s.Empty(foundTests)

	s.IsType(gimlet.ErrorResponse{}, err)
	apiErr, ok := err.(gimlet.ErrorResponse)
	s.True(ok)
	s.Equal(http.StatusNotFound, apiErr.StatusCode)
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommitAndStatus() {
	for _, status := range []string{"pass", "fail"} {
		for pix := 0; pix < s.numProjects; pix++ {
			for cix := 0; cix < s.numCommits; cix++ {
				opts := task.GetTasksByProjectAndCommitOptions{
					Project:        fmt.Sprintf("project_%d", pix),
					CommitHash:     fmt.Sprintf("commit_%d", cix),
					StartingTaskId: "",
					Status:         status,
					TaskName:       "",
					VariantName:    "",
					Limit:          0,
				}
				foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
				s.NoError(err)
				s.Len(foundTasks, s.numTasks/2)
				for _, t := range foundTasks {
					s.Equal(status, t.Status)
				}
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommitAndVariant() {
	for _, variant := range []string{"bv1", "bv2"} {
		for pix := 0; pix < s.numProjects; pix++ {
			for cix := 0; cix < s.numCommits; cix++ {
				opts := task.GetTasksByProjectAndCommitOptions{
					Project:        fmt.Sprintf("project_%d", pix),
					CommitHash:     fmt.Sprintf("commit_%d", cix),
					StartingTaskId: "",
					Status:         "",
					TaskName:       "",
					VariantName:    variant,
					Limit:          0,
				}
				foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
				s.NoError(err)
				s.Len(foundTasks, s.numTasks/2)
				for _, t := range foundTasks {
					s.Equal(variant, t.BuildVariant)
				}
			}
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindByProjectAndCommitAndTaskName() {
	for pix := 0; pix < s.numProjects; pix++ {
		for cix := 0; cix < s.numCommits; cix++ {
			for tix := 0; tix < s.numTasks; tix++ {
				opts := task.GetTasksByProjectAndCommitOptions{
					Project:        fmt.Sprintf("project_%d", pix),
					CommitHash:     fmt.Sprintf("commit_%d", cix),
					StartingTaskId: "",
					Status:         "",
					TaskName:       fmt.Sprintf("task_%d", tix),
					VariantName:    "",
					Limit:          0,
				}
				foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
				s.NoError(err)
				s.Len(foundTasks, 1)
				for _, t := range foundTasks {
					s.Equal(fmt.Sprintf("task_%d", tix), t.DisplayName)
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
		opts := task.GetTasksByProjectAndCommitOptions{
			Project:        projectId,
			CommitHash:     commitId,
			StartingTaskId: tids[i],
			Status:         "",
			TaskName:       "",
			VariantName:    "",
			Limit:          0,
		}
		foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
		s.NoError(err)

		startAt := 0

		s.Len(foundTasks, (s.numTasks-startAt)-i)
		for ix, t := range foundTasks {
			index := ix + i
			s.Equal(tids[index], t.Id)
		}
	}

}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindWithLimit() {
	commitId := "commit_0"
	projectId := "project_0"
	limit := 2
	tids := s.taskIds[0][0]
	for i := 0; i < s.numTasks/limit; i++ {
		index := i * limit
		taskName := tids[index]
		opts := task.GetTasksByProjectAndCommitOptions{
			Project:        projectId,
			CommitHash:     commitId,
			StartingTaskId: taskName,
			Status:         "",
			TaskName:       "",
			VariantName:    "",
			Limit:          limit,
		}
		foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
		s.NoError(err)
		s.Len(foundTasks, limit)
		for ix, t := range foundTasks {
			s.Equal(tids[ix+index], t.Id)
		}
	}
}

func (s *TaskConnectorFetchByProjectAndCommitSuite) TestFindEmptyProjectAndCommit() {
	projectId := "project_0"
	commitId := "commit_0"
	opts := task.GetTasksByProjectAndCommitOptions{
		Project:        projectId,
		CommitHash:     commitId,
		StartingTaskId: "",
		Status:         "",
		TaskName:       "",
		VariantName:    "",
		Limit:          1,
	}
	foundTasks, err := FindTasksByProjectAndCommit(s.ctx, opts)
	s.NoError(err)
	s.Len(foundTasks, 1)
	task1 := foundTasks[0]
	s.Equal(s.taskIds[0][0][0], task1.Id)
}

func TestCheckTaskSecret(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))

	task := task.Task{
		Id:     "task1",
		Secret: "abcdef",
	}
	assert.NoError(task.Insert(t.Context()))

	r := &http.Request{
		Header: http.Header{
			evergreen.TaskHeader: []string{"task1"},
		},
	}
	dbTask, code, err := CheckTaskSecret("task1", r)
	assert.Error(err)
	assert.Equal(http.StatusConflict, code)
	assert.Nil(dbTask)

	r.Header.Set(evergreen.TaskSecretHeader, "abcdef")
	dbTask, code, err = CheckTaskSecret("task1", r)
	assert.NoError(err)
	assert.Equal(http.StatusOK, code)
	assert.NotNil(dbTask)

}
