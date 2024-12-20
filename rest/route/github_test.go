package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	s.NotEmpty(s.env.Settings().GithubWebhookSecret)

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

	s.rm = makeGithubHooksRoute(s.sc, s.queue, []byte(s.conf.GithubWebhookSecret), s.env.Settings())
	s.mockRm = makeGithubHooksRoute(s.mockSc, s.queue, []byte(s.conf.GithubWebhookSecret), s.env.Settings())

	s.Require().NoError(commitqueue.InsertQueue(&commitqueue.CommitQueue{ProjectID: "mci"}))

	var err error
	s.prBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Len(s.prBody, 24692)
	s.pushBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "push_event.json"))
	s.NoError(err)
	s.Len(s.pushBody, 7597)
	s.commitQueueCommentBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "commit_queue_comment_event.json"))
	s.NoError(err)
	s.Len(s.commitQueueCommentBody, 11494)
	s.retryCommentBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "retry_comment_event.json"))
	s.NoError(err)
	s.Len(s.retryCommentBody, 11468)
	s.patchCommentBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "patch_comment_event.json"))
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
		Enabled:          true,
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
	secret := []byte(s.conf.GithubWebhookSecret)
	req, err := makeRequest("1", "pull_request", s.prBody, secret)
	s.NoError(err)
	req.Header.Del("X-Hub-Signature")

	err = s.h.Parse(ctx, req)
	s.Equal("pull_request", s.h.eventType)
	s.Error(err)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidate() {
	ctx := context.Background()
	secret := []byte(s.conf.GithubWebhookSecret)
	req, err := makeRequest("1", "pull_request", s.prBody, secret)
	req = setGitHubPayload(req, s.prBody)
	s.NoError(err)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("pull_request", s.h.eventType)
	s.Equal("1", s.h.msgID)

	req, err = makeRequest("2", "push", s.pushBody, secret)
	req = setGitHubPayload(req, s.pushBody)
	s.NoError(err)
	s.NotNil(req)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("push", s.h.eventType)
	s.Equal("2", s.h.msgID)

	req, err = makeRequest("3", "issue_comment", s.commitQueueCommentBody, secret)
	req = setGitHubPayload(req, s.commitQueueCommentBody)
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
		Enabled: true,
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
		Enabled: true,
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
	s.True(triggersStatusRefresh(refreshStatusComment))
	s.False(triggersStatusRefresh(retryComment))

	//test whitespace trimming
	s.True(triggersStatusRefresh("  evergreen refresh "))
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

	// test whitespace trimming
	s.True(triggersPatch("  evergreen patch "))

	// test --alias behavior
	s.True(triggersPatch(commentString))
	commentString = "  evergreen patch --alias patch-alias "
	s.True(triggersPatch(commentString))
	s.Equal(patch.ManualCaller, parsePRCommentForCaller(commentString))
	s.Equal("patch-alias", parsePRCommentForAlias(commentString))
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	s.False(triggersCommitQueue(comment))

	comment = commitQueueMergeComment
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

	v, err := s.mock.createVersionForTag(context.Background(), pRef, nil, model.Revision{}, tag)
	s.NoError(err)
	s.NotNil(v)
}

func TestGetHelpTextFromProjects(t *testing.T) {
	cqAndPREnabledProject := model.ProjectRef{
		Id:      "cqEnabled",
		Enabled: true,
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.TruePtr(),
		},
		PRTestingEnabled: utility.TruePtr(),
	}
	manualPRProject := model.ProjectRef{
		Id:                     "manualEnabled",
		Enabled:                true,
		ManualPRTestingEnabled: utility.TruePtr(),
	}
	cqDisabledWithTextProject := model.ProjectRef{
		Id:      "cqDisabled",
		Enabled: true,
		CommitQueue: model.CommitQueueParams{
			Enabled: utility.FalsePtr(),
			Message: "this commit queue isn't enabled",
		},
	}
	disabledProject := model.ProjectRef{
		Id:      "disabled",
		Enabled: false,
	}
	repoRefWithPRTesting := &model.RepoRef{ProjectRef: model.ProjectRef{
		Id:                     "enabled",
		PRTestingEnabled:       utility.TruePtr(),
		ManualPRTestingEnabled: utility.TruePtr(),
	}}
	repoRefWithoutPRTesting := &model.RepoRef{ProjectRef: model.ProjectRef{
		Id:               "notEnabled",
		PRTestingEnabled: nil,
	}}

	for testCase, test := range map[string]func(*testing.T){
		"cqEnabledAndDisabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{cqAndPREnabledProject, cqDisabledWithTextProject}
			helpText := getHelpTextFromProjects(nil, pRefs)
			assert.Contains(t, helpText, refreshStatusComment)
			assert.Contains(t, helpText, commitQueueMergeComment)
			assert.NotContains(t, helpText, cqDisabledWithTextProject.CommitQueue.Message)
			assert.Contains(t, helpText, retryComment)
			assert.NotContains(t, helpText, patchComment)
		},
		"manualAndAutomaticPRTestingEnabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{cqAndPREnabledProject, manualPRProject}
			helpText := getHelpTextFromProjects(nil, pRefs)
			assert.Contains(t, helpText, refreshStatusComment)
			assert.Contains(t, helpText, commitQueueMergeComment)
			assert.Contains(t, helpText, patchComment)
			assert.Contains(t, helpText, retryComment)
		},
		"manualPRTestingEnabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{manualPRProject}
			helpText := getHelpTextFromProjects(nil, pRefs)
			assert.Contains(t, helpText, refreshStatusComment)
			assert.Contains(t, helpText, patchComment)
			assert.NotContains(t, helpText, retryComment)
			assert.NotContains(t, helpText, commitQueueMergeComment)
		},
		"cqDisabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{cqDisabledWithTextProject}
			helpText := getHelpTextFromProjects(nil, pRefs)
			assert.Contains(t, helpText, commitQueueMergeComment)
			assert.Contains(t, helpText, cqDisabledWithTextProject.CommitQueue.Message)
			assert.NotContains(t, helpText, refreshStatusComment)
			assert.NotContains(t, helpText, retryComment)
			assert.NotContains(t, helpText, patchComment)
		},
		"repoPRTestingEnabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{cqDisabledWithTextProject}
			helpText := getHelpTextFromProjects(repoRefWithPRTesting, pRefs)
			assert.Contains(t, helpText, commitQueueMergeComment)
			assert.Contains(t, helpText, cqDisabledWithTextProject.CommitQueue.Message)
			assert.Contains(t, helpText, refreshStatusComment)
			assert.Contains(t, helpText, patchComment)
			assert.Contains(t, helpText, retryComment)
		},
		"repoNoTestingEnabled": func(t *testing.T) {
			pRefs := []model.ProjectRef{cqDisabledWithTextProject}
			helpText := getHelpTextFromProjects(repoRefWithoutPRTesting, pRefs)
			assert.Contains(t, helpText, commitQueueMergeComment)
			assert.Contains(t, helpText, cqDisabledWithTextProject.CommitQueue.Message)
			assert.NotContains(t, helpText, refreshStatusComment)
			assert.NotContains(t, helpText, patchComment)
			assert.NotContains(t, helpText, retryComment)
		},
		"repoPRTestingNotUsed": func(t *testing.T) {
			// We only use repo PR testing if there are no disabled projects.
			pRefs := []model.ProjectRef{disabledProject}
			helpText := getHelpTextFromProjects(repoRefWithPRTesting, pRefs)
			assert.NotContains(t, helpText, refreshStatusComment)
			assert.NotContains(t, helpText, patchComment)
			assert.NotContains(t, helpText, retryComment)
			assert.NotContains(t, helpText, commitQueueMergeComment)
		},
	} {

		t.Run(testCase, test)
	}
}

func TestPRDef(t *testing.T) {
	assert.NoError(t, db.Clear(patch.Collection))
	owner := "owner"
	repo := "repo"
	patchId := mgobson.ObjectIdHex("5aeb4514f27e4f9984646d97")
	p := &patch.Patch{
		Id:      patchId,
		Project: "mci",
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:               5,
			BaseOwner:              owner,
			BaseRepo:               repo,
			RepeatPatchIdNextPatch: patchId.Hex(),
		},
	}
	assert.NoError(t, p.Insert())
	err := keepPRPatchDefinition(owner, repo, 5)
	assert.NoError(t, err)

	p, err = patch.FindOne(patch.ById(patchId))
	assert.NoError(t, err)
	assert.Equal(t, patchId.Hex(), p.GithubPatchData.RepeatPatchIdNextPatch)

	err = resetPRPatchDefinition(owner, repo, 5)
	assert.NoError(t, err)

	p, err = patch.FindOne(patch.ById(patchId))
	assert.NoError(t, err)
	assert.Equal(t, "", p.GithubPatchData.RepeatPatchIdNextPatch)

}

func TestHandleGitHubMergeGroup(t *testing.T) {
	org := "evergreen-ci"
	repo := "evergreen"
	branch := "main"
	baseRef := "refs/heads/main"
	baseSha := "d2a90288ad96adca4a7d0122d8d4fd1deb24db11"
	headRef := "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
	trueBool := true
	event := &github.MergeGroupEvent{
		Org: &github.Organization{
			Login: &org,
		},
		Repo: &github.Repository{
			Name: &repo,
		},
		MergeGroup: &github.MergeGroup{
			BaseRef: &baseRef,
			BaseSHA: &baseSha,
			HeadRef: &headRef,
		},
	}
	gh := &githubHookApi{}
	p := model.ProjectRef{
		Owner:   org,
		Repo:    repo,
		Branch:  branch,
		Enabled: true,
		CommitQueue: model.CommitQueueParams{
			Enabled: &trueBool,
		},
	}
	for testCase, test := range map[string]func(*testing.T){
		"githubMergeQueueSelected": func(t *testing.T) {
			p.CommitQueue.MergeQueue = model.MergeQueueGitHub
			require.NoError(t, p.Insert())
			response := gh.handleMergeGroupChecksRequested(event)
			// check for error returned by GitHub merge queue handler
			str := fmt.Sprintf("%#v", response)
			assert.Contains(t, str, "message ID cannot be empty")
			assert.NotContains(t, str, "200")
		},
		"nonexistentProject": func(t *testing.T) {
			response := gh.handleMergeGroupChecksRequested(event)
			// check for error returned by GitHub merge queue handler
			str := fmt.Sprintf("%#v", response)
			assert.Contains(t, str, "no matching project ref")
			assert.NotContains(t, str, "200")
		},
	} {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
		t.Run(testCase, test)
	}
}
