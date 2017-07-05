package route

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
		PushTime:     timeField,
		Version:      versionId,
		BuildVariant: s.bv[0],
	}
	testBuild2 := build.Build{
		Id:           s.bi[1],
		CreateTime:   timeField,
		StartTime:    timeField,
		FinishTime:   timeField,
		PushTime:     timeField,
		Version:      versionId,
		BuildVariant: s.bv[1],
	}

	s.versionData = data.MockVersionConnector{
		CachedVersions: []version.Version{testVersion1},
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
	handler := &versionHandler{versionId: "versionId"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(1, len(res.Result))
	version := res.Result[0]
	h, ok := (version).(*model.APIVersion)
	s.True(ok)
	s.Equal(model.APIString(versionId), h.Id)
	s.Equal(model.APITime(timeField), h.CreateTime)
	s.Equal(model.APITime(timeField), h.StartTime)
	s.Equal(model.APITime(timeField), h.FinishTime)
	s.Equal(model.APIString(revision), h.Revision)
	s.Equal(model.APIString(author), h.Author)
	s.Equal(model.APIString(authorEmail), h.AuthorEmail)
	s.Equal(model.APIString(msg), h.Message)
	s.Equal(model.APIString(status), h.Status)
	s.Equal(model.APIString(repo), h.Repo)
	s.Equal(model.APIString(branch), h.Branch)
}

// TestFindAllBuildsForVersion tests the route for finding all builds for a version.
func (s *VersionSuite) TestFindAllBuildsForVersion() {
	handler := &buildsForVersionHandler{versionId: "versionId"}
	res, err := handler.Execute(nil, s.sc)
	s.NoError(err)
	s.NotNil(res)
	s.Equal(2, len(res.Result))

	for idx, build := range res.Result {
		b, ok := (build).(*model.APIBuild)
		s.True(ok)
		s.Equal(model.APIString(s.bi[idx]), b.Id)
		s.Equal(model.APITime(timeField), b.CreateTime)
		s.Equal(model.APITime(timeField), b.StartTime)
		s.Equal(model.APITime(timeField), b.FinishTime)
		s.Equal(model.APITime(timeField), b.PushTime)
		s.Equal(model.APIString(versionId), b.Version)
		s.Equal(model.APIString(s.bv[idx]), b.BuildVariant)
	}
}
