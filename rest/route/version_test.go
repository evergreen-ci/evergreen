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
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	bv, bi []string // build variants and build indices for testing
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

	s.NoError(db.ClearCollections(task.Collection, serviceModel.VersionCollection, build.Collection))
	s.bv = append(s.bv, "buildvariant1", "buildvariant2")
	s.bi = append(s.bi, "buildId1", "buildId2")

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
	}
	testBuild2 := build.Build{
		Id:           s.bi[1],
		Version:      versionId,
		BuildVariant: s.bv[1],
	}
	versions := []serviceModel.Version{testVersion1}

	tasks := []task.Task{
		{Id: "task1", Version: versionId, Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task2", Version: versionId, Aborted: false, Status: evergreen.TaskDispatched},
		{Id: "task3", Version: versionId, Aborted: false, Status: evergreen.TaskUndispatched, BuildId: testBuild1.Id},
	}

	builds := []build.Build{testBuild1, testBuild2}

	for _, item := range versions {
		s.Require().NoError(item.Insert())
	}
	for _, item := range tasks {
		s.Require().NoError(item.Insert())
	}
	for _, item := range builds {
		s.Require().NoError(item.Insert())
	}
}

func (s *VersionSuite) TearDownSuite() {
	s.cancel()
}

// TestFindByVersionId tests the route for finding version by its ID.
func (s *VersionSuite) TestFindByVersionId() {
	handler := &versionHandler{versionId: "versionId"}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	s.Equal(utility.ToStringPtr(revision), h.Revision)
	s.Equal(utility.ToStringPtr(author), h.Author)
	s.Equal(utility.ToStringPtr(authorEmail), h.AuthorEmail)
	s.Equal(utility.ToStringPtr(msg), h.Message)
	s.Equal(utility.ToStringPtr(status), h.Status)
	s.Equal(utility.ToStringPtr(repo), h.Repo)
	s.Equal(utility.ToStringPtr(branch), h.Branch)
	s.Equal(utility.ToStringPtr(project), h.Project)
}

func (s *VersionSuite) TestPatchVersionVersion() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "caller1"})
	handler := &versionPatchHandler{
		versionId: versionId,
		Activated: utility.TruePtr(),
	}

	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	v, err := serviceModel.VersionFindOneId(versionId)
	s.Nil(err)
	for _, b := range v.BuildIds {
		build, err := build.FindOneId(b)
		s.Nil(err)
		s.Equal(true, build.Activated)
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
	res := handler.Run(context.TODO())
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	builds, ok := res.Data().([]model.APIBuild)
	s.True(ok)

	s.Len(builds, 2)

	for idx, b := range builds {
		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
	}
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
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())
	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)

	// Check that all tasks have been aborted.
	tasks, err := task.FindAllTaskIDsFromVersion("versionId")
	s.NoError(err)
	for _, t := range tasks {
		foundTask, err := task.FindOneId(t)
		s.NoError(err)
		if utility.StringSliceContains(evergreen.TaskAbortableStatuses, foundTask.Status) {
			s.Equal(foundTask.Aborted, true)
		}
	}
}

// TestRestartVersion tests the route for restarting a version.
func (s *VersionSuite) TestRestartVersion() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "caller1"})

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
	v, err := serviceModel.VersionFindOneId("versionId")
	s.NoError(err)
	s.Equal(evergreen.VersionStarted, v.Status)
}
