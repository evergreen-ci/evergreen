package units

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
	"gopkg.in/mgo.v2/bson"
)

type githubStatusUpdateSuite struct {
	suite.Suite
	patchDoc *patch.Patch
	buildDoc *build.Build
	cancel   func()
}

func TestGithubStatusUpdate(t *testing.T) {
	suite.Run(t, new(githubStatusUpdateSuite))
}

func (s *githubStatusUpdateSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
}

func (s *githubStatusUpdateSuite) TearDownSuite() {
	s.cancel()
}

func (s *githubStatusUpdateSuite) SetupTest() {
	evergreen.ResetEnvironment()

	s.NoError(db.ClearCollections(patch.Collection))
	startTime := time.Now()
	s.patchDoc = &patch.Patch{
		Id:         bson.NewObjectId(),
		Version:    bson.NewObjectId().Hex(),
		Status:     evergreen.PatchFailed,
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		GithubPatchData: patch.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "tychoish",
			HeadRepo:  "evergreen",
			PRNumber:  448,
			HeadHash:  "776f608b5b12cd27b8d931c8ee4ca0c13f857299",
		},
	}

	s.buildDoc = &build.Build{
		Id:           bson.NewObjectId().Hex(),
		BuildVariant: "testvariant",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildFailed,
	}

	s.NoError(s.patchDoc.Insert())
	s.NoError(s.buildDoc.Insert())
}

func (s *githubStatusUpdateSuite) TearDownTest() {
	evergreen.ResetEnvironment()
}

func (s *githubStatusUpdateSuite) TestFetchRejectsBadStatuses() {
	job, ok := NewGithubStatusUpdateJobForPatch(s.patchDoc).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.GHStatus = "cheese"

	s.Error(job.fetch())

	job.Run()
	s.Error(job.Error())
	s.True(job.Status().Completed)
}

func (s *githubStatusUpdateSuite) TestFetchForBuildPopulatesRepoInfo() {
	job, ok := NewGithubStatusUpdateJobForBuild(s.buildDoc).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.VersionID = s.patchDoc.Version

	s.NoError(job.fetch())
	s.Equal("evergreen-ci", job.Owner)
	s.Equal("evergreen", job.Repo)
	s.Equal(448, job.PRNumber)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", job.Ref)
}

func (s *githubStatusUpdateSuite) TestForBuild() {
	job, ok := NewGithubStatusUpdateJobForBuild(s.buildDoc).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)

	s.NoError(job.fetch())

	s.Equal(s.patchDoc.Version, job.VersionID)
	s.Equal("evergreen-ci", job.Owner)
	s.Equal("evergreen", job.Repo)
	s.Equal(448, job.PRNumber)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", job.Ref)

	s.Equal(fmt.Sprintf("/build/%s", s.buildDoc.Id), job.URLPath)
	s.Equal("no tasks were run", job.Description)
	s.Equal("evergreen-testvariant", job.Context)
	s.Equal("failure", job.GHStatus)
}

func (s *githubStatusUpdateSuite) TestForPatch() {
	s.NoError(db.ClearCollections(patch.Collection))

	job, ok := NewGithubStatusUpdateJobForPatch(s.patchDoc).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)

	s.NoError(job.fetch())

	s.Empty(job.VersionID)
	s.Equal("evergreen-ci", job.Owner)
	s.Equal("evergreen", job.Repo)
	s.Equal(448, job.PRNumber)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", job.Ref)

	s.Equal(fmt.Sprintf("/version/%s", s.patchDoc.Version), job.URLPath)
	s.Equal("finished in 10m0s", job.Description)
	s.Equal("evergreen", job.Context)
	s.Equal("failure", job.GHStatus)
}

func (s *githubStatusUpdateSuite) TestWithGithub() {
	// We always skip this test b/c Github's API only lets the status of a
	// ref be set 1000 times, and doesn't allow status removal (so runnning
	// this test in the suite will fail after the 1000th time).
	// It's still useful for manual testing
	s.T().Skip("Github Status API is limited")
	testutil.ConfigureIntegrationTest(s.T(), testConfig, "TestWithGithub")
	evergreen.GetEnvironment().Settings().Credentials = testConfig.Credentials
	evergreen.GetEnvironment().Settings().Ui.Url = "http://example.com"

	s.patchDoc.GithubPatchData.BaseRepo = "sample"
	s.patchDoc.GithubPatchData.HeadRepo = "sample"
	s.patchDoc.GithubPatchData.HeadOwner = "richardsamuels"
	s.patchDoc.GithubPatchData.PRNumber = 1
	s.patchDoc.GithubPatchData.HeadHash = "de724e67df25f1d5fb22102df5ce55baf439209c"

	job, ok := NewGithubStatusUpdateJobForPatch(s.patchDoc).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.Run()
	s.NoError(job.Error())

	githubOauthToken, err := evergreen.GetEnvironment().Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	token := strings.Split(githubOauthToken, " ")
	s.Require().Len(token, 2)
	s.Require().Equal("token", token[0])

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token[1]},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	statuses, _, err := client.Repositories.ListStatuses(ctx, s.patchDoc.GithubPatchData.BaseOwner,
		s.patchDoc.GithubPatchData.BaseRepo, s.patchDoc.GithubPatchData.HeadHash, nil)
	s.Require().NoError(err)
	s.Require().NotEmpty(statuses)

	lastStatus := statuses[0]
	s.Require().NotNil(lastStatus)
	s.Require().NotNil(lastStatus.State)
	s.Require().NotNil(lastStatus.Description)
	s.Require().NotNil(lastStatus.Context)
	s.Require().NotNil(lastStatus.TargetURL)

	s.Equal(githubStatusFailure, *lastStatus.State)
	s.Equal("finished in 10m0s", *lastStatus.Description)
	s.Equal("evergreen", *lastStatus.Context)
	s.Equal(fmt.Sprintf("http://example.com%s", job.URLPatch), *lastStatus.TargetURL)
}
