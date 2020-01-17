package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	sc          *data.MockConnector
	versionData data.MockVersionConnector
	buildData   data.MockBuildConnector
	bv, bi      []string // build variants and build indices for testing

	suite.Suite
}

func TestVersionSuite(t *testing.T) {
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

	s.versionData = data.MockVersionConnector{
		CachedVersions: []serviceModel.Version{testVersion1},
		CachedTasks: []task.Task{
			{Version: versionId, Aborted: false, Status: evergreen.TaskStarted},
			{Version: versionId, Aborted: false, Status: evergreen.TaskDispatched},
		},
		CachedRestartedVersions: make(map[string]string),
	}
	s.buildData = data.MockBuildConnector{
		CachedBuilds: []build.Build{testBuild1, testBuild2},
	}
	s.sc = &data.MockConnector{
		MockVersionConnector: s.versionData,
		MockBuildConnector:   s.buildData,
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
	s.Equal(model.ToStringPtr(versionId), h.Id)
	s.Equal(timeField, h.CreateTime)
	s.Equal(timeField, h.StartTime)
	s.Equal(timeField, h.FinishTime)
	s.Equal(model.ToStringPtr(revision), h.Revision)
	s.Equal(model.ToStringPtr(author), h.Author)
	s.Equal(model.ToStringPtr(authorEmail), h.AuthorEmail)
	s.Equal(model.ToStringPtr(msg), h.Message)
	s.Equal(model.ToStringPtr(status), h.Status)
	s.Equal(model.ToStringPtr(repo), h.Repo)
	s.Equal(model.ToStringPtr(branch), h.Branch)
	s.Equal(model.ToStringPtr(project), h.Project)
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

		s.Equal(model.ToStringPtr(s.bi[idx]), b.Id)
		s.Equal(timeField, b.CreateTime)
		s.Equal(timeField, b.StartTime)
		s.Equal(timeField, b.FinishTime)
		s.Equal(model.ToStringPtr(versionId), b.Version)
		s.Equal(model.ToStringPtr(s.bv[idx]), b.BuildVariant)
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

		s.Equal(versionId, model.FromStringPtr(b.Version))
		s.Equal(s.bi[i], model.FromStringPtr(b.Id))
		s.Equal(variant, model.FromStringPtr(b.BuildVariant))
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
	s.Equal(model.ToStringPtr(versionId), h.Id)

	// Check that all tasks have been aborted.
	for _, t := range s.versionData.CachedTasks {
		s.Equal(t.Aborted, true)
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
	s.Equal(model.ToStringPtr(versionId), h.Id)
	s.Equal("caller1", s.versionData.CachedRestartedVersions["versionId"])
}
