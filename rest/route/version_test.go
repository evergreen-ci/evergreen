package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	sc          *data.DBConnector
	versionData data.DBVersionConnector
	buildData   data.DBBuildConnector
	bv, bi      []string // build variants and build indices for testing

	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, new(VersionSuite))
}

// Declare field variables to use across routes related to version.
var timeField time.Time
var versionId, revision, author, authorEmail, msg, status, repo, branch, project string

// SetupSuite sets up the test suite for routes related to version.
// More version-related routes will be implemented later.
func (s *VersionSuite) SetupSuite() {
	// Initialize values for version field variables.
	timeField = time.Now()
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
		CreateTime:    timeField,
		StartTime:     timeField,
		FinishTime:    timeField,
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
		CreateTime:   timeField,
		StartTime:    timeField,
		FinishTime:   timeField,
		Version:      versionId,
		BuildVariant: s.bv[0],
	}
	testBuild2 := build.Build{
		Id:           s.bi[1],
		CreateTime:   timeField,
		StartTime:    timeField,
		FinishTime:   timeField,
		Version:      versionId,
		BuildVariant: s.bv[1],
	}

	s.versionData = data.DBVersionConnector{}
	s.sc = &data.DBConnector{
		DBVersionConnector: s.versionData,
		DBBuildConnector:   s.buildData,
	}
	versions := []serviceModel.Version{testVersion1}

	tasks := []task.Task{
		{Version: versionId, Aborted: false, Status: evergreen.TaskStarted},
		{Version: versionId, Aborted: false, Status: evergreen.TaskDispatched},
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

// TestFindByVersionId tests the route for finding version by its ID.
func (s *VersionSuite) TestFindByVersionId() {
	handler := &versionHandler{versionId: "versionId", sc: s.sc}
	res := handler.Run(context.TODO())
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	h, ok := res.Data().(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	s.Equal(timeField, *h.CreateTime)
	s.Equal(timeField, *h.StartTime)
	s.Equal(timeField, *h.FinishTime)
	s.Equal(utility.ToStringPtr(revision), h.Revision)
	s.Equal(utility.ToStringPtr(author), h.Author)
	s.Equal(utility.ToStringPtr(authorEmail), h.AuthorEmail)
	s.Equal(utility.ToStringPtr(msg), h.Message)
	s.Equal(utility.ToStringPtr(status), h.Status)
	s.Equal(utility.ToStringPtr(repo), h.Repo)
	s.Equal(utility.ToStringPtr(branch), h.Branch)
	s.Equal(utility.ToStringPtr(project), h.Project)
}

// TestFindAllBuildsForVersion tests the route for finding all builds for a version.
func (s *VersionSuite) TestFindAllBuildsForVersion() {
	handler := &buildsForVersionHandler{versionId: "versionId", sc: s.sc}
	res := handler.Run(context.TODO())
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	builds, ok := res.Data().([]model.Model)
	s.True(ok)

	s.Len(builds, 2)

	for idx, build := range builds {
		b, ok := (build).(*model.APIBuild)
		s.True(ok)

		s.Equal(utility.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(timeField, *b.CreateTime)
		s.Equal(timeField, *b.StartTime)
		s.Equal(timeField, *b.FinishTime)
		s.Equal(utility.ToStringPtr(versionId), b.Version)
		s.Equal(utility.ToStringPtr(s.bv[idx]), b.BuildVariant)
	}
}

func (s *VersionSuite) TestFindBuildsForVersionByVariant() {
	handler := &buildsForVersionHandler{versionId: "versionId", sc: s.sc}

	for i, variant := range s.bv {
		handler.variant = variant
		res := handler.Run(context.Background())
		s.Equal(http.StatusOK, res.Status())
		s.NotNil(res)

		builds, ok := res.Data().([]model.Model)
		s.True(ok)

		s.Require().Len(builds, 1)
		b, ok := (builds[0]).(*model.APIBuild)
		s.True(ok)

		s.Equal(versionId, utility.FromStringPtr(b.Version))
		s.Equal(s.bi[i], utility.FromStringPtr(b.Id))
		s.Equal(variant, utility.FromStringPtr(b.BuildVariant))
	}
}

// TestAbortVersion tests the route for aborting a version.
func (s *VersionSuite) TestAbortVersion() {
	handler := &versionAbortHandler{versionId: "versionId", sc: s.sc}

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
		s.Equal(foundTask.Aborted, true)
	}
}

// TestRestartVersion tests the route for restarting a version.
func (s *VersionSuite) TestRestartVersion() {
	ctx := context.Background()
	ctx = gimlet.AttachUser(ctx, &user.DBUser{Id: "caller1"})

	handler := &versionRestartHandler{versionId: "versionId", sc: s.sc}

	// Check that Execute runs without error and returns
	// the correct Version.
	res := handler.Run(ctx)
	s.NotNil(res)
	s.Equal(http.StatusOK, res.Status())

	version := res.Data()
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(utility.ToStringPtr(versionId), h.Id)
	v, err := s.versionData.FindVersionById("versionId")
	s.NoError(err)
	s.Equal("caller1", v.Requester)
}
