package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	bv, bi []string            // build variants and build indices for testing
	btc    [][]build.TaskCache // build task caches for testing
	env    evergreen.Environment
	ctx    context.Context
	cancel context.CancelFunc

	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(VersionSuite))
}

// Declare field variables to use across routes related to version.
var versionId, revision, author, authorEmail, msg, status, repo, branch, project string

// SetupSuite sets up the test suite for routes related to version.
// More version-related routes will be implemented later.
func (s *VersionSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	env := &mock.Environment{}
	s.Require().NoError(env.Configure(s.ctx))
	s.env = env

	// Initialize values for version field variables.
	versionId = "versionId"
	revision = "revision"
	author = "author"
	authorEmail = "author_email"
	msg = "message"
	status = "status"
	repo = "repo"
	branch = "branch"
	project = "project"

	s.NoError(db.ClearCollections(task.Collection, serviceModel.VersionCollection, build.Collection, manifest.Collection))
	s.bv = append(s.bv, "buildvariant1", "buildvariant2")
	s.bi = append(s.bi, "buildId1", "buildId2")
	s.btc = [][]build.TaskCache{
		{{Id: "task1"}, {Id: "task3"}},
		{{Id: "task2"}},
	}

	// Initialize fields for a test model.Version
	buildVariants := []serviceModel.VersionBuildStatus{
		{
			BuildVariant: s.bv[0],
			BuildId:      s.bi[0],
		},
		{
			BuildVariant: s.bv[1],
			BuildId:      s.bi[1],
		},
	}
	testVersion1 := serviceModel.Version{
		Id:            versionId,
		Revision:      revision,
		Author:        author,
		AuthorEmail:   authorEmail,
		Message:       msg,
		Status:        status,
		Repo:          repo,
		Branch:        branch,
		BuildVariants: buildVariants,
		Identifier:    project,
	}

	testBuild1 := build.Build{
		Id:           s.bi[0],
		Version:      versionId,
		BuildVariant: s.bv[0],
		Tasks:        s.btc[0],
	}
	testBuild2 := build.Build{
		Id:           s.bi[1],
		Version:      versionId,
		BuildVariant: s.bv[1],
		Tasks:        s.btc[1],
	}
	versions := []serviceModel.Version{testVersion1}

	tasks := []task.Task{
		{Id: "task1", Version: versionId, Aborted: false, Status: evergreen.TaskStarted, BuildId: testBuild1.Id},
		{Id: "task2", Version: versionId, Aborted: false, Status: evergreen.TaskDispatched, BuildId: testBuild2.Id},
		{Id: "task3", Version: versionId, Aborted: false, Status: evergreen.TaskUndispatched, BuildId: testBuild1.Id},
	}

	builds := []build.Build{testBuild1, testBuild2}

	for _, item := range versions {
		s.Require().NoError(item.Insert(s.ctx))
	}
	for _, item := range tasks {
		s.Require().NoError(item.Insert(s.ctx))
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert(s.ctx))
	}
}

func (s *VersionSuite) TearDownSuite() {
	s.cancel()
}

// TestFindByVersionId tests the route for finding version by its ID.
func (s *VersionSuite) TestFindByVersionId() {
	handler := &versionHandler{versionId: "versionId"}
	res := handler.Run(s.ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	s.Equal(utility.ToStringPtr(revision), h.Revision)
	s.Equal(utility.ToStringPtr(author), h.Author)
	s.Equal(utility.ToStringPtr(authorEmail), h.AuthorEmail)
	s.Equal(utility.ToStringPtr(msg), h.Message)
	// Status may change during other tests, so check if it's either the original status or started
	s.True(utility.FromStringPtr(h.Status) == status || utility.FromStringPtr(h.Status) == evergreen.VersionStarted)
	s.Equal(utility.ToStringPtr(repo), h.Repo)
	s.Equal(utility.ToStringPtr(branch), h.Branch)
	s.Equal(utility.ToStringPtr(project), h.Project)
}

func (s *VersionSuite) TestPatchVersionVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})
	handler := &versionPatchHandler{
		versionId: versionId,
		Activated: utility.TruePtr(),
	}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	v, err := serviceModel.VersionFindOneId(s.ctx, versionId)
	s.NoError(err)
	for _, b := range v.BuildIds {
		build, err := build.FindOneId(s.ctx, b)
		s.NoError(err)
		s.True(build.Activated)
	}
	s.Equal(versionId, v.Id)
	s.Equal(revision, v.Revision)
	s.Equal(author, v.Author)
	s.Equal(authorEmail, v.AuthorEmail)
	s.Equal(msg, v.Message)
	s.Equal(evergreen.VersionStarted, v.Status)
	s.Equal(repo, v.Repo)
	s.Equal(branch, v.Branch)
	s.Equal(utility.TruePtr(), v.Activated)
}

// TestFindAllBuildsForVersion tests the route for finding all builds for a version.
func (s *VersionSuite) TestFindAllBuildsForVersion() {
	handler := &buildsForVersionHandler{
		versionId: "versionId",
		env:       s.env,
	}
	res := handler.Run(s.ctx)
	s.Equal(http.StatusOK, res.Status())
	s.Require().NotNil(res)

	builds, ok := res.Data().([]model.APIBuild)
	s.Require().True(ok)

	s.Require().Len(builds, 2)

	for idx, b := range builds {
		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
		s.Zero(b.TaskCache, "task info should not be populated by default")
		s.Zero(b.StatusCounts, "task info should not be populated by default")
	}
}

func (s *VersionSuite) TestFindAllBuildsForVersionWithTaskStatuses() {
	handler := &buildsForVersionHandler{
		versionId:       "versionId",
		env:             s.env,
		includeTaskInfo: true,
	}
	res := handler.Run(s.ctx)
	s.Equal(http.StatusOK, res.Status())
	s.Require().NotNil(res)

	builds, ok := res.Data().([]model.APIBuild)
	s.Require().True(ok)

	s.Require().Len(builds, 2)

	for idx, b := range builds {
		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
		s.Require().Len(b.TaskCache, len(s.btc[idx]))
	}
	s.Equal(task.TaskStatusCount{Started: 1, Undispatched: 1}, builds[0].StatusCounts)
	s.Equal(task.TaskStatusCount{Started: 1}, builds[1].StatusCounts)
}

func (s *VersionSuite) TestFindBuildsForVersionByVariant() {
	handler := &buildsForVersionHandler{
		versionId: "versionId",
		env:       s.env,
	}

	for i, variant := range s.bv {
		handler.variant = variant
		res := handler.Run(s.ctx)
		s.Equal(http.StatusOK, res.Status())
		s.NotNil(res)

		builds, ok := res.Data().([]model.APIBuild)
		s.True(ok)

		s.Require().Len(builds, 1)
		s.Equal(versionId, utility.FromStringPtr(builds[0].Version))
		s.Equal(s.bi[i], utility.FromStringPtr(builds[0].Id))
		s.Equal(variant, utility.FromStringPtr(builds[0].BuildVariant))
	}
}

// TestAbortVersion tests the route for aborting a version.
func (s *VersionSuite) TestAbortVersion() {
	handler := &versionAbortHandler{versionId: "versionId"}

	// Check that Execute runs without error and returns
	// the correct Version.
	res := handler.Run(s.ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())
	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)

	// Check that all tasks have been aborted.
	tasks, err := task.FindAllTaskIDsFromVersion(s.ctx, "versionId")
	s.NoError(err)
	for _, t := range tasks {
		foundTask, err := task.FindOneId(s.ctx, t)
		s.NoError(err)
		if utility.StringSliceContains(evergreen.TaskInProgressStatuses, foundTask.Status) {
			s.True(foundTask.Aborted)
		}
	}
}

// TestRestartVersion tests the route for restarting a version.
func (s *VersionSuite) TestRestartVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionRestartHandler{versionId: "versionId"}

	// Check that Execute runs without error and returns
	// the correct Version.
	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	v, err := serviceModel.VersionFindOneId(s.ctx, "versionId")
	s.NoError(err)
	s.Equal(evergreen.VersionStarted, v.Status)
}

// TestActivateVersionTasks tests the route for activating specific tasks in a version.
func (s *VersionSuite) TestActivateVersionTasks() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	testTasks := []task.Task{
		{
			Id:           "inactive_task1",
			Version:      versionId,
			BuildVariant: s.bv[0],
			DisplayName:  "test_task_1",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[0],
		},
		{
			Id:           "inactive_task2",
			Version:      versionId,
			BuildVariant: s.bv[0],
			DisplayName:  "test_task_2",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[0],
		},
		{
			Id:           "inactive_task3",
			Version:      versionId,
			BuildVariant: s.bv[1],
			DisplayName:  "test_task_3",
			Activated:    false,
			Status:       evergreen.TaskUndispatched,
			BuildId:      s.bi[1],
		},
	}

	for _, task := range testTasks {
		s.Require().NoError(task.Insert(s.ctx))
	}

	handler := &versionActivateTasksHandler{
		versionId: versionId,
		Variants: []Variant{
			{
				Name:  s.bv[0],
				Tasks: []string{"test_task_1", "test_task_2"},
			},
			{
				Name:  s.bv[1],
				Tasks: []string{"test_task_3"},
			},
		},
	}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	// Test successful activation
	// Verify tasks were activated
	activatedTask1, err := task.FindOneId(s.ctx, "inactive_task1")
	s.NoError(err)
	s.True(activatedTask1.Activated)

	activatedTask2, err := task.FindOneId(s.ctx, "inactive_task2")
	s.NoError(err)
	s.True(activatedTask2.Activated)

	activatedTask3, err := task.FindOneId(s.ctx, "inactive_task3")
	s.NoError(err)
	s.True(activatedTask3.Activated)
}

// TestActivateVersionTasksInvalidVariant tests error handling for invalid variant/task combinations.
func (s *VersionSuite) TestActivateVersionTasksInvalidVariant() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionActivateTasksHandler{
		versionId: versionId,
		Variants: []Variant{
			{
				Name:  s.bv[0],
				Tasks: []string{"test_task_1", "test_task_2"},
			},
		},
	}

	// Test with non-existent variant/tasks
	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusBadRequest, res.Status())
}

func (s *VersionSuite) TestGetManifestForVersion() {
	mfst := manifest.Manifest{
		Id:          versionId,
		Revision:    revision,
		ProjectName: project,
		Branch:      branch,
		Modules: map[string]*manifest.Module{
			"module1": {
				Branch:   "module1_branch",
				Repo:     "module1_repo",
				Revision: "module1_revision",
				Owner:    "module1_owner",
				URL:      "module1_url",
			},
		},
	}
	exists, err := mfst.TryInsert(s.ctx)
	s.Require().NoError(err)
	s.False(exists)

	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionManifestGetHandler{versionId: "versionId"}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	apiMfst, ok := res.Data().(*model.APIManifest)
	s.Require().True(ok)
	s.Equal(versionId, utility.FromStringPtr(apiMfst.Id))
	s.Equal(revision, utility.FromStringPtr(apiMfst.Revision))
	s.Equal(project, utility.FromStringPtr(apiMfst.ProjectName))
	s.Equal(branch, utility.FromStringPtr(apiMfst.Branch))
	s.NotEmpty(apiMfst.Modules)
	s.Len(apiMfst.Modules, 1)
	expectedModule := mfst.Modules["module1"]
	s.Equal("module1", utility.FromStringPtr(apiMfst.Modules[0].Name))
	s.Equal(expectedModule.Branch, utility.FromStringPtr(apiMfst.Modules[0].Branch))
	s.Equal(expectedModule.Repo, utility.FromStringPtr(apiMfst.Modules[0].Repo))
	s.Equal(expectedModule.Revision, utility.FromStringPtr(apiMfst.Modules[0].Revision))
	s.Equal(expectedModule.Owner, utility.FromStringPtr(apiMfst.Modules[0].Owner))
	s.Equal(expectedModule.URL, utility.FromStringPtr(apiMfst.Modules[0].URL))
}

func (s *VersionSuite) TestGetManifestForVersionErrorsForNonexistentVersion() {
	ctx := gimlet.AttachUser(s.ctx, &user.DBUser{Id: "caller1"})

	handler := &versionManifestGetHandler{versionId: "nonexistent"}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusNotFound, res.Status())
}
