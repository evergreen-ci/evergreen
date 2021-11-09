package route

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
)

type GithubWebhookRouteSuite struct {
	sc                     *data.MockConnector
	rm                     gimlet.RouteHandler
	canceler               context.CancelFunc
	conf                   *evergreen.Settings
	prBody                 []byte
	pushBody               []byte
	commitQueueCommentBody []byte
	retryCommentBody       []byte
	h                      *githubHookApi
	queue                  amboy.Queue
	env                    evergreen.Environment
	suite.Suite
}

func (s *GithubWebhookRouteSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel

	s.env = testutil.NewEnvironment(ctx, s.T())
	s.NotNil(s.env.Settings())
	s.NotNil(s.env.Settings().Api)
	s.NotEmpty(s.env.Settings().Api.GithubWebhookSecret)

	s.conf = testutil.TestConfig()
	s.NotNil(s.conf)
}

func (s *GithubWebhookRouteSuite) TearDownSuite() {
	s.canceler()
}

func (s *GithubWebhookRouteSuite) SetupTest() {
	grip.Critical(s.conf.Api)

	s.NoError(db.Clear(model.ProjectRefCollection))
	s.NoError(db.Clear(commitqueue.Collection))

	s.queue = evergreen.GetEnvironment().LocalQueue()
	s.sc = &data.MockConnector{
		MockPatchIntentConnector: data.MockPatchIntentConnector{
			CachedIntents: map[data.MockPatchIntentKey]patch.Intent{},
		},
		MockProjectConnector: data.MockProjectConnector{
			CachedProjects: []model.ProjectRef{
				model.ProjectRef{
					Id:          "bth",
					Enabled:     utility.TruePtr(),
					Owner:       "baxterthehacker",
					Repo:        "public-repo",
					Branch:      "main",
					CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
				},
			},
		},
		MockCommitQueueConnector: data.MockCommitQueueConnector{
			Queue: map[string][]restModel.APICommitQueueItem{
				"bth": []restModel.APICommitQueueItem{},
			},
			UserPermissions: map[data.UserRepoInfo]string{
				data.UserRepoInfo{
					Username: "baxterthehacker",
					Owner:    "baxterthehacker",
					Repo:     "public-repo",
				}: "write",
			},
		},
	}

	s.rm = makeGithubHooksRoute(s.sc, s.queue, []byte(s.conf.Api.GithubWebhookSecret), evergreen.GetEnvironment().Settings())

	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Len(s.prBody, 24731)
	s.pushBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "push_event.json"))
	s.NoError(err)
	s.Len(s.pushBody, 7597)
	s.commitQueueCommentBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "commit_queue_comment_event.json"))
	s.NoError(err)
	s.Len(s.commitQueueCommentBody, 11494)
	s.retryCommentBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "retry_comment_event.json"))
	s.NoError(err)
	s.Len(s.retryCommentBody, 11468)

	var ok bool
	s.h, ok = s.rm.Factory().(*githubHookApi)
	s.True(ok)
}

func TestGithubWebhookRouteSuite(t *testing.T) {
	s := new(GithubWebhookRouteSuite)
	suite.Run(t, s)
}

func (s *GithubWebhookRouteSuite) TestAddIntent() {
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)

	s.h.event = event
	s.h.msgID = "1"

	ctx := context.Background()
	resp := s.h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 1)
}

func (s *GithubWebhookRouteSuite) TestAddDuplicateIntentFails() {
	s.TestAddIntent()

	ctx := context.Background()

	resp := s.h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 1)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidateFailsWithoutSignature() {
	ctx := context.Background()
	secret := []byte(s.conf.Api.GithubWebhookSecret)
	req, err := makeRequest("1", "pull_request", s.prBody, secret)
	s.NoError(err)
	req.Header.Del("X-Hub-Signature")

	err = s.h.Parse(ctx, req)
	s.Equal("pull_request", s.h.eventType)
	s.Error(err)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidate() {
	ctx := context.Background()
	secret := []byte(s.conf.Api.GithubWebhookSecret)
	req, err := makeRequest("1", "pull_request", s.prBody, secret)
	s.NoError(err)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("pull_request", s.h.eventType)
	s.Equal("1", s.h.msgID)

	req, err = makeRequest("2", "push", s.pushBody, secret)
	s.NoError(err)
	s.NotNil(req)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("push", s.h.eventType)
	s.Equal("2", s.h.msgID)

	req, err = makeRequest("3", "issue_comment", s.commitQueueCommentBody, secret)
	s.NoError(err)
	s.NotNil(req)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("issue_comment", s.h.eventType)
	s.Equal("3", s.h.msgID)
}

func makeRequest(uid, event string, body, secret []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", "http://example.com/rest/v2/hooks/github", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	signature, err := util.CalculateHMACHash(secret, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-type", "application/json")
	req.Header.Add("X-Github-Event", event)
	req.Header.Add("X-GitHub-Delivery", uid)
	req.Header.Add("X-Hub-Signature", signature)
	return req, nil
}

func (s *GithubWebhookRouteSuite) TestPushEventTriggersRepoTracker() {
	ref := &model.ProjectRef{
		Id:      "meh",
		Enabled: utility.TruePtr(),
		Owner:   "baxterthehacker",
		Repo:    "public-repo",
		Branch:  "changes",
	}
	s.Require().NoError(ref.Insert())
	event, err := github.ParseWebHook("push", s.pushBody)
	s.NotNil(event)
	s.NoError(err)

	s.h.event = event
	s.h.msgID = "1"

	ctx := context.Background()

	resp := s.h.Run(ctx)
	if s.NotNil(resp) {
		s.Equal(http.StatusOK, resp.Status())
	}
}

func (s *GithubWebhookRouteSuite) TestCommitQueueCommentTrigger() {
	event, err := github.ParseWebHook("issue_comment", s.commitQueueCommentBody)
	s.NotNil(event)
	s.NoError(err)
	s.h.event = event
	s.h.msgID = "1"
	ctx := context.Background()
	resp := s.h.Run(ctx)
	if s.NotNil(resp) {
		s.Equal(http.StatusOK, resp.Status())
	}

	s.NoError(err)
	if s.Len(s.sc.MockCommitQueueConnector.Queue, 1) {
		s.Equal("1", utility.FromStringPtr(s.sc.MockCommitQueueConnector.Queue["bth"][0].Issue))
		s.Equal("test_module", utility.FromStringPtr(s.sc.MockCommitQueueConnector.Queue["bth"][0].Modules[0].Module))
		s.Equal("1234", utility.FromStringPtr(s.sc.MockCommitQueueConnector.Queue["bth"][0].Modules[0].Issue))
	}
}

func (s *GithubWebhookRouteSuite) TestRetryCommentTrigger() {
	event, err := github.ParseWebHook("issue_comment", s.retryCommentBody)
	s.NoError(err)
	s.NotNil(event)

	issueComment, ok := event.(*github.IssueCommentEvent)
	s.True(ok)
	commentString := issueComment.Comment.GetBody()
	s.Equal(retryComment, commentString)

	s.True(triggersRetry("created", commentString))
	s.False(triggersRetry("deleted", commentString))

	//test whitespace trimming
	s.True(triggersRetry("created", "  evergreen retry "))
}

func (s *GithubWebhookRouteSuite) TestUnknownEventType() {
	var emptyPayload []byte
	event, err := github.ParseWebHook("unknown_type", emptyPayload)
	s.Error(err)
	s.Nil(event)

	s.h.event = event
	ctx := context.Background()
	resp := s.h.Run(ctx)
	if s.NotNil(resp) {
		s.Equal(http.StatusOK, resp.Status())
	}
}

func (s *GithubWebhookRouteSuite) TestTryDequeueCommitQueueItemForPR() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, commitqueue.Collection))

	owner := "baxterthehacker"
	repo := "public-repo"
	branch := "main"
	number := 1

	fillerString := "a"
	fillerBool := false
	pr := &github.PullRequest{
		MergeCommitSHA: &fillerString,
		User:           &github.User{Login: &fillerString},
		Base: &github.PullRequestBranch{
			Repo: &github.Repository{
				Owner: &github.User{
					Login: &owner,
				},
				Name:     &repo,
				FullName: &fillerString,
			},
			Ref: &branch,
			SHA: &fillerString,
		},
		Head:    &github.PullRequestBranch{SHA: &fillerString},
		Title:   &fillerString,
		HTMLURL: &fillerString,
		Merged:  &fillerBool,
	}

	// try dequeue errors if the PR is missing information (PR number)
	s.Error(s.h.tryDequeueCommitQueueItemForPR(pr))

	// try dequeue returns no error if there is no matching item
	newNumber := 2
	pr.Number = &newNumber
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))

	pr.Number = &number
	// try dequeue returns no errors if there is no matching queue
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))

	// try dequeue works when an item matches
	_, err := s.sc.EnqueueItem("bth", restModel.APICommitQueueItem{Issue: utility.ToStringPtr("1")}, false)
	s.NoError(err)
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))
	queue, err := s.sc.FindCommitQueueForProject("bth")
	s.NoError(err)
	s.Empty(queue.Queue)

	// try dequeue returns no error if no projectRef matches the PR
	owner = "octocat"
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))
}

func (s *GithubWebhookRouteSuite) TestCreateVersionForTag() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection))

	tag := model.GitTag{
		Tag:    "release",
		Pusher: "release-bot",
	}
	s.sc.Aliases = []restModel.APIProjectAlias{
		{
			Alias:      utility.ToStringPtr(evergreen.GitTagAlias),
			GitTag:     utility.ToStringPtr("release"),
			RemotePath: utility.ToStringPtr("rest/route/testdata/release.yml"),
		},
	}
	pRef := model.ProjectRef{
		Id:                    "my-project",
		GitTagAuthorizedUsers: []string{"release-bot", "not-release-bot"},
		GitTagVersionsEnabled: utility.TruePtr(),
	}
	v, err := s.h.createVersionForTag(context.Background(), pRef, nil, model.Revision{}, tag, "")
	s.NoError(err)
	s.NotNil(v)
}
