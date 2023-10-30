package units

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
)

type githubStatusUpdateSuite struct {
	env      *mock.Environment
	patchDoc *patch.Patch
	buildDoc *build.Build
	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context

	suite.Suite
}

func TestGithubStatusUpdate(t *testing.T) {
	s := &githubStatusUpdateSuite{}
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)

	suite.Run(t, s)
}

func (s *githubStatusUpdateSuite) TearDownSuite() {
	s.cancel()
}

func (s *githubStatusUpdateSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())
	s.NoError(db.ClearCollections(patch.Collection, patch.IntentCollection, model.ProjectRefCollection, evergreen.ConfigCollection))

	uiConfig := evergreen.UIConfig{}
	uiConfig.Url = "https://example.com"
	s.Require().NoError(uiConfig.Set(s.ctx))

	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(s.ctx))

	startTime := time.Now().Truncate(time.Millisecond)
	id := mgobson.NewObjectId()
	s.patchDoc = &patch.Patch{
		Id:         id,
		Version:    id.Hex(),
		Status:     evergreen.VersionFailed,
		StartTime:  startTime,
		FinishTime: startTime.Add(10 * time.Minute),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "tychoish",
			HeadRepo:  "evergreen",
			PRNumber:  448,
			HeadHash:  "776f608b5b12cd27b8d931c8ee4ca0c13f857299",
		},
	}

	s.buildDoc = &build.Build{
		Id:           mgobson.NewObjectId().Hex(),
		BuildVariant: "testvariant",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildFailed,
	}

	s.NoError(s.patchDoc.Insert())
	s.NoError(s.buildDoc.Insert())
}

func (s *githubStatusUpdateSuite) TestRunInDegradedMode() {
	flags := evergreen.ServiceFlags{
		GithubStatusAPIDisabled: true,
	}
	s.Require().NoError(evergreen.SetServiceFlags(s.ctx, flags))

	job, ok := NewGithubStatusUpdateJobForNewPatch(s.patchDoc.Version).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)

	s.Error(job.Error())
	s.Contains(job.Error().Error(), "GitHub status updates are disabled, not updating status")
}

func (s *githubStatusUpdateSuite) TestForPatchCreated() {
	s.NoError(db.ClearCollections(patch.Collection))
	s.patchDoc.Status = evergreen.VersionCreated
	s.NoError(s.patchDoc.Insert())

	job, ok := NewGithubStatusUpdateJobForNewPatch(s.patchDoc.Version).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().Equal(githubUpdateTypeNewPatch, job.UpdateType)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.msgToStatus(s.env.InternalSender)

	s.Equal("evergreen-ci", status.Owner)
	s.Equal("evergreen", status.Repo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", status.Ref)

	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("preparing to run tasks", status.Description)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
}

func (s *githubStatusUpdateSuite) TestForPushToCommitQueue() {
	owner, repo, ref := "evergreen-ci", "evergreen", "776f608b5b12cd27b8d931c8ee4ca0c13f857299"
	prNum := 1
	job := NewGithubStatusUpdateJobForPushToCommitQueue(owner, repo, ref, prNum, "").(*githubStatusUpdateJob)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.msgToStatus(s.env.InternalSender)

	s.Equal(owner, status.Owner)
	s.Equal(repo, status.Repo)
	s.Equal(ref, status.Ref)

	s.Zero(status.URL)
	s.Equal(commitqueue.GithubContext, status.Context)
	s.Equal("added to queue", status.Description)
	s.Equal(message.GithubStatePending, status.State)
}

func (s *githubStatusUpdateSuite) TestForDeleteFromCommitQueue() {
	owner, repo, ref := "evergreen-ci", "evergreen", "776f608b5b12cd27b8d931c8ee4ca0c13f857299"
	prNum := 1
	job := NewGithubStatusUpdateJobForDeleteFromCommitQueue(owner, repo, ref, prNum).(*githubStatusUpdateJob)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.msgToStatus(s.env.InternalSender)

	s.Equal(owner, status.Owner)
	s.Equal(repo, status.Repo)
	s.Equal(ref, status.Ref)

	s.Zero(status.URL)
	s.Equal(commitqueue.GithubContext, status.Context)
	s.Equal("removed from queue", status.Description)
	s.Equal(message.GithubStateSuccess, status.State)
}

func (s *githubStatusUpdateSuite) TestForProcessingError() {
	intent, err := patch.NewGithubIntent("1", "", "", testutil.NewGithubPR(448,
		"evergreen-ci/evergreen", "7c38f3f63c05675329518c148d3a176e1da6ec2d", "tychoish/evergreen", "776f608b5b12cd27b8d931c8ee4ca0c13f857299", "tychoish", "Title"))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	job, ok := NewGithubStatusUpdateJobForProcessingError("evergreen/commit-queue", "evergreen-ci", "evergreen", "776f608b5b12cd27b8d931c8ee4ca0c13f857299", OtherErrors).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().Equal(githubUpdateTypeProcessingError, job.UpdateType)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.msgToStatus(s.env.InternalSender)

	s.Equal("evergreen-ci", status.Owner)
	s.Equal("evergreen", status.Repo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", status.Ref)
	s.Equal(OtherErrors, status.Description)
	s.Equal("evergreen/commit-queue", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
}

func (s *githubStatusUpdateSuite) TestRequestForAuth() {
	s.NoError(db.ClearCollections(patch.Collection))
	s.patchDoc.Status = evergreen.VersionCreated
	s.NoError(s.patchDoc.Insert())

	job, ok := NewGithubStatusUpdateJobForExternalPatch(s.patchDoc.Version).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().Equal(githubUpdateTypeRequestAuth, job.UpdateType)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.msgToStatus(s.env.InternalSender)

	s.Equal("evergreen-ci", status.Owner)
	s.Equal("evergreen", status.Repo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", status.Ref)

	s.Equal(fmt.Sprintf("https://example.com/patch/%s", s.patchDoc.Version), status.URL)
	s.Equal("patch must be manually authorized", status.Description)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
}

func (s *githubStatusUpdateSuite) msgToStatus(sender *send.InternalSender) *message.GithubStatus {
	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	raw := msg.Message
	s.Require().NotNil(raw)
	status, ok := raw.Raw().(*message.GithubStatus)
	s.Require().True(ok)

	return status
}

func (s *githubStatusUpdateSuite) TestPreamble() {
	j := makeGithubStatusUpdateJob()
	j.env = s.env
	s.Require().NotNil(j)
	s.NoError(j.preamble(s.ctx))
	s.NotNil(j.env)
	s.NotEmpty(j.urlBase)
	s.Equal(s.env, j.env)

	uiConfig := evergreen.UIConfig{}
	s.NoError(uiConfig.Set(s.ctx))

	s.EqualError(j.preamble(s.ctx), "UI URL is empty")
}

func (s *githubStatusUpdateSuite) TestWithGithub() {
	// We always skip this test b/c Github's API only lets the status of a
	// ref be set 1000 times, and doesn't allow status removal (so runnning
	// this test in the suite will fail after the 1000th time).
	// It's still useful for manual testing
	s.T().Skip("Github Status API is limited")

	env := testutil.NewEnvironment(s.ctx, s.T())
	settings := testutil.TestConfig()

	testutil.ConfigureIntegrationTest(s.T(), settings, "TestWithGithub")
	env.Settings().Credentials = settings.Credentials
	env.Settings().Ui.Url = "http://example.com"

	s.patchDoc.GithubPatchData.BaseRepo = "sample"
	s.patchDoc.GithubPatchData.HeadOwner = "richardsamuels"
	s.patchDoc.GithubPatchData.HeadRepo = "sample"
	s.patchDoc.GithubPatchData.PRNumber = 1
	s.patchDoc.GithubPatchData.HeadHash = "de724e67df25f1d5fb22102df5ce55baf439209c"

	s.NoError(db.ClearCollections(patch.Collection))
	s.NoError(s.patchDoc.Insert())

	job, ok := NewGithubStatusUpdateJobForNewPatch(s.patchDoc.Version).(*githubStatusUpdateJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.Run(s.ctx)
	s.NoError(job.Error())

	githubOauthToken, err := evergreen.GetEnvironment().Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubOauthToken},
	)
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
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

	s.Equal("failure", *lastStatus.State)
	s.Equal("finished in 10m0s", *lastStatus.Description)
	s.Equal("evergreen", *lastStatus.Context)
	s.Equal(fmt.Sprintf("http://example.com/version/%s", s.patchDoc.Version), *lastStatus.TargetURL)
}
