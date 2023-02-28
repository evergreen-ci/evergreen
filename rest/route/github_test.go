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
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type GithubWebhookRouteSuite struct {
	sc                     *data.DBConnector
	mockSc                 *data.MockGitHubConnector
	rm                     gimlet.RouteHandler
	mockRm                 gimlet.RouteHandler
	canceler               context.CancelFunc
	conf                   *evergreen.Settings
	prBody                 []byte
	pushBody               []byte
	commitQueueCommentBody []byte
	retryCommentBody       []byte
	patchCommentBody       []byte
	h                      *githubHookApi
	mock                   *githubHookApi
	queue                  amboy.Queue
	env                    evergreen.Environment
	suite.Suite
}

func (s *GithubWebhookRouteSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel

	env := &mock.Environment{}
	s.Require().NoError(env.Configure(ctx))
	s.env = env
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
	s.NoError(db.Clear(model.ProjectRefCollection))
	s.NoError(db.Clear(commitqueue.Collection))

	s.queue = s.env.LocalQueue()
	s.mockSc = &data.MockGitHubConnector{
		MockGitHubConnectorImpl: data.MockGitHubConnectorImpl{},
	}

	s.rm = makeGithubHooksRoute(s.sc, s.queue, []byte(s.conf.Api.GithubWebhookSecret), s.env.Settings())
	s.mockRm = makeGithubHooksRoute(s.mockSc, s.queue, []byte(s.conf.Api.GithubWebhookSecret), s.env.Settings())

	s.Require().NoError(commitqueue.InsertQueue(&commitqueue.CommitQueue{ProjectID: "mci"}))

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
	s.patchCommentBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "patch_comment_event.json"))
	s.NoError(err)
	s.Len(s.patchCommentBody, 11468)

	var ok bool
	s.h, ok = s.rm.Factory().(*githubHookApi)
	s.True(ok)
	s.mock, ok = s.mockRm.Factory().(*githubHookApi)
	s.True(ok)
}

func TestGithubWebhookRouteSuite(t *testing.T) {
	s := new(GithubWebhookRouteSuite)

	suite.Run(t, s)
}

func (s *GithubWebhookRouteSuite) TestIsItemOnCommitQueue() {
	pos, err := data.EnqueueItem("mci", restModel.APICommitQueueItem{Source: utility.ToStringPtr(commitqueue.SourceDiff), Issue: utility.ToStringPtr("1")}, false)
	s.Require().NoError(err)
	s.Require().Equal(0, pos)

	exists, err := isItemOnCommitQueue("mci", "1")
	s.NoError(err)
	s.True(exists)

	exists, err = isItemOnCommitQueue("mci", "2")
	s.NoError(err)
	s.False(exists)

	exists, err = isItemOnCommitQueue("not-a-project", "1")
	s.Error(err)
	s.False(exists)
}

func (s *GithubWebhookRouteSuite) TestAddIntentAndFailsWithDuplicate() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, patch.IntentCollection))

	doc := &model.ProjectRef{
		Owner:            "baxterthehacker",
		Repo:             "public-repo",
		Branch:           "main",
		Enabled:          utility.TruePtr(),
		BatchTime:        10,
		Id:               "ident0",
		PRTestingEnabled: utility.TruePtr(),
	}
	s.NoError(doc.Insert())
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)

	s.h.event = event
	s.h.msgID = "1"

	ctx := context.Background()
	resp := s.h.Run(ctx)
	s.Equal(http.StatusOK, resp.Status())
	count, err := db.CountQ(patch.IntentCollection, db.Query(bson.M{}))
	s.NoError(err)
	s.Equal(count, 1)

	resp = s.h.Run(ctx)
	s.NotEqual(http.StatusOK, resp.Status())
	count, err = db.CountQ(patch.IntentCollection, db.Query(bson.M{}))
	s.NoError(err)
	s.Equal(count, 1)
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
	req, err := http.NewRequest(http.MethodPost, "http://example.com/rest/v2/hooks/github", bytes.NewReader(body))
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
	s.NoError(db.ClearCollections(model.ProjectRefCollection, commitqueue.Collection))
	cq := &commitqueue.CommitQueue{ProjectID: "proj"}
	s.NoError(commitqueue.InsertQueue(cq))
	p := model.ProjectRef{
		Id:      "proj",
		Owner:   "baxterthehacker",
		Repo:    "public-repo",
		Branch:  "main",
		Enabled: utility.TruePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	s.NoError(p.Insert())
	event, err := github.ParseWebHook("issue_comment", s.commitQueueCommentBody)
	args1 := data.UserRepoInfo{
		Username: "baxterthehacker",
		Owner:    "baxterthehacker",
		Repo:     "public-repo",
	}
	s.mockSc.MockGitHubConnectorImpl.UserPermissions = map[data.UserRepoInfo]string{
		args1: "admin",
	}
	s.NotNil(event)
	s.NoError(err)
	s.mock.event = event
	s.mock.msgID = "1"
	ctx := context.Background()
	resp := s.mock.Run(ctx)
	if s.NotNil(resp) {
		s.Equal(http.StatusOK, resp.Status())
	}

	s.NoError(err)
	cq, err = commitqueue.FindOneId("proj")
	s.NoError(err)
	if s.Len(cq.Queue, 1) {
		s.Equal("1", utility.FromStringPtr(&cq.Queue[0].Issue))
		s.Equal("test_module", utility.FromStringPtr(&cq.Queue[0].Modules[0].Module))
		s.Equal("1234", utility.FromStringPtr(&cq.Queue[0].Modules[0].Issue))
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

	s.True(triggersPatch(commentString))

	//test whitespace trimming
	s.True(triggersPatch("  evergreen retry "))
}

func (s *GithubWebhookRouteSuite) TestRefreshStatusTrigger() {
	s.True(triggersPatch(refreshStatusComment))
	s.False(triggersPatch(retryComment))

	//test whitespace trimming
	s.True(triggersPatch("  evergreen refresh "))
}

func (s *GithubWebhookRouteSuite) TestPatchCommentTrigger() {
	event, err := github.ParseWebHook("issue_comment", s.patchCommentBody)
	s.NoError(err)
	s.NotNil(event)

	issueComment, ok := event.(*github.IssueCommentEvent)
	s.True(ok)
	commentString := issueComment.Comment.GetBody()
	s.Equal(patchComment, commentString)

	s.True(triggersPatch(commentString))

	//test whitespace trimming
	s.True(triggersPatch("  evergreen patch "))
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	s.False(triggersCommitQueue(comment))

	comment = triggerComment
	s.True(triggersCommitQueue(comment))
}

func (s *CommitQueueSuite) TestCommentCleanup() {
	trigger := " \n Evergreen       \n  Merge \n It's me, hi, I'm the comment it's me \n "
	patch := " \n Evergreen       \n  Patch \n "
	retry := " \n Evergreen       \n  Retry \n "

	s.False(triggersCommitQueue(patch))
	s.False(isPatchComment(retry))
	s.False(isRetryComment(trigger))

	s.True(triggersCommitQueue(trigger))
	s.True(isPatchComment(patch))
	s.True(isRetryComment(retry))
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
	projectRef := &model.ProjectRef{
		Id:      "bth",
		Owner:   "baxterthehacker",
		Repo:    "public-repo",
		Branch:  "main",
		Enabled: utility.TruePtr(),
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
	}
	s.NoError(projectRef.Insert())
	cq := &commitqueue.CommitQueue{ProjectID: "bth"}
	s.NoError(commitqueue.InsertQueue(cq))

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
	_, err := data.EnqueueItem("bth", restModel.APICommitQueueItem{Issue: utility.ToStringPtr("1")}, false)
	s.NoError(err)
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))
	queue, err := data.FindCommitQueueForProject("bth")
	s.NoError(err)
	s.Empty(queue.Queue)

	// try dequeue returns no error if no projectRef matches the PR
	owner = "octocat"
	s.NoError(s.h.tryDequeueCommitQueueItemForPR(pr))
}

func (s *GithubWebhookRouteSuite) TestCreateVersionForTag() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, model.ProjectAliasCollection))
	tag := model.GitTag{
		Tag:    "release",
		Pusher: "release-bot",
	}
	s.mockSc.Aliases = []restModel.APIProjectAlias{
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
	projectAlias := model.ProjectAlias{
		ProjectID:  "my-project",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "rest/route/testdata/release.yml",
	}
	s.NoError(pRef.Insert())
	s.NoError(projectAlias.Upsert())

	v, err := s.mock.createVersionForTag(context.Background(), pRef, nil, model.Revision{}, tag, "")
	s.NoError(err)
	s.NotNil(v)
}
