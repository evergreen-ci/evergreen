package route

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

// VersionSuite enables testing for version related routes.
type VersionSuite struct {
	sc   *data.MockConnector
	data data.MockVersionConnector

	suite.Suite
}

func TestVersionSuite(t *testing.T) {
	suite.Run(t, new(VersionSuite))
}

// Declare field variables to use across routes related to version.
var timeField time.Time
var versionId, revision, author, authorEmail, msg, status, repo, branch string
var bv1, bv2, bi1, bi2 string

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

	bv1 = "buildvariant1"
	bv2 = "buildvariant2"
	bi1 = "buildId1"
	bi2 = "buildId2"

	// Initialize fields for a test version.Version
	buildVariants := []version.BuildStatus{
		{
			BuildVariant: bv1,
			BuildId:      bi1,
		},
		{
			BuildVariant: bv2,
			BuildId:      bi2,
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

	s.data = data.MockVersionConnector{
		CachedVersions: []version.Version{testVersion1},
	}
	s.sc = &data.MockConnector{
		MockVersionConnector: s.data,
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
