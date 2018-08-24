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
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
)

type GithubWebhookRouteSuite struct {
	sc       *data.MockConnector
	rm       gimlet.RouteHandler
	canceler context.CancelFunc
	conf     *evergreen.Settings
	prBody   []byte
	pushBody []byte
	h        *githubHookApi
	queue    amboy.Queue
	suite.Suite
}

func (s *GithubWebhookRouteSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	s.canceler = cancel
	err := evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil)
	s.NoError(err)
	s.NotNil(evergreen.GetEnvironment().Settings())
	s.NotNil(evergreen.GetEnvironment().Settings().Api)
	s.NotEmpty(evergreen.GetEnvironment().Settings().Api.GithubWebhookSecret)

	s.conf = testutil.TestConfig()
	s.NotNil(s.conf)
}

func (s *GithubWebhookRouteSuite) TearDownSuite() {
	s.canceler()
	evergreen.ResetEnvironment()
}

func (s *GithubWebhookRouteSuite) SetupTest() {
	grip.Critical(s.conf.Api)

	s.NoError(db.Clear(model.ProjectRefCollection))

	s.queue = evergreen.GetEnvironment().LocalQueue()
	s.sc = &data.MockConnector{MockPatchIntentConnector: data.MockPatchIntentConnector{
		CachedIntents: map[data.MockPatchIntentKey]patch.Intent{},
	}}

	s.rm = makeGithubHooksRoute(s.sc, s.queue, []byte(s.conf.Api.GithubWebhookSecret))

	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))

	s.NoError(err)
	s.Len(s.prBody, 24743)
	s.pushBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "push_event.json"))
	s.NoError(err)
	s.Len(s.pushBody, 7603)

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
	req, err := makeRequest("1", s.prBody, secret)
	s.NoError(err)
	req.Header.Del("X-Hub-Signature")

	err = s.h.Parse(ctx, req)
	s.Equal("pull_request", s.h.eventType)
	s.Error(err)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidate() {
	ctx := context.Background()
	secret := []byte(s.conf.Api.GithubWebhookSecret)
	req, err := makeRequest("1", s.prBody, secret)
	s.NoError(err)

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("pull_request", s.h.eventType)
	s.Equal("1", s.h.msgID)

	req, err = makeRequest("2", s.pushBody, secret)
	s.NoError(err)
	s.NotNil(req)
	req.Header.Del("X-Github-Event")
	req.Header.Add("X-Github-Event", "push")

	err = s.h.Parse(ctx, req)
	s.NoError(err)
	s.NotNil(s.h.event)
	s.Equal("push", s.h.eventType)
	s.Equal("2", s.h.msgID)
}

func makeRequest(uid string, body, secret []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", "http://example.com/rest/v2/hooks/github", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	signature, err := util.CalculateHMACHash(secret, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-type", "application/json")
	req.Header.Add("X-Github-Event", "pull_request")
	req.Header.Add("X-GitHub-Delivery", uid)
	req.Header.Add("X-Hub-Signature", signature)
	return req, nil
}

func (s *GithubWebhookRouteSuite) TestPushEventTriggersRepoTracker() {
	ref := &model.ProjectRef{
		Identifier: "meh",
		Enabled:    true,
		Owner:      "baxterthehacker",
		Repo:       "public-repo",
		Branch:     "changes",
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
