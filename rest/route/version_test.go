package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
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
var versionId, revision, author, authorEmail, msg, status, repo, branch string

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

	s.bv = append(s.bv, "buildvariant1", "buildvariant2")
	s.bi = append(s.bi, "buildId1", "buildId2")

	// Initialize fields for a test version.Version
	buildVariants := []version.BuildStatus{
		{
			BuildVariant: s.bv[0],
			BuildId:      s.bi[0],
		},
		{
			BuildVariant: s.bv[1],
			BuildId:      s.bi[1],
		},
	}
	testVersion1 := version.Version{
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
		CachedVersions: []version.Version{testVersion1},
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
	s.Equal(model.ToAPIString(versionId), h.Id)
	s.Equal(model.NewTime(timeField), h.CreateTime)
	s.Equal(model.NewTime(timeField), h.StartTime)
	s.Equal(model.NewTime(timeField), h.FinishTime)
	s.Equal(model.ToAPIString(revision), h.Revision)
	s.Equal(model.ToAPIString(author), h.Author)
	s.Equal(model.ToAPIString(authorEmail), h.AuthorEmail)
	s.Equal(model.ToAPIString(msg), h.Message)
	s.Equal(model.ToAPIString(status), h.Status)
	s.Equal(model.ToAPIString(repo), h.Repo)
	s.Equal(model.ToAPIString(branch), h.Branch)
}

// TestFindAllBuildsForVersion tests the route for finding all builds for a version.
func (s *VersionSuite) TestFindAllBuildsForVersion() {
	handler := &buildsForVersionHandler{versionId: "versionId", sc: s.sc}
	res := handler.Run(context.TODO())
	s.Equal(http.StatusOK, res.Status())
	s.NotNil(res)

	builds, ok := res.Data().([]model.Model)
	s.True(ok)

	s.Equal(2, len(builds))

	for idx, build := range builds {
		b, ok := (build).(*model.APIBuild)
		s.True(ok)

		s.Equal(model.ToAPIString(s.bi[idx]), b.Id)
		s.Equal(model.NewTime(timeField), b.CreateTime)
		s.Equal(model.NewTime(timeField), b.StartTime)
		s.Equal(model.NewTime(timeField), b.FinishTime)
		s.Equal(model.ToAPIString(versionId), b.Version)
		s.Equal(model.ToAPIString(s.bv[idx]), b.BuildVariant)
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
	s.Equal(model.ToAPIString(versionId), h.Id)

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
	s.Equal(model.ToAPIString(versionId), h.Id)
	s.Equal("caller1", s.versionData.CachedRestartedVersions["versionId"])
}
