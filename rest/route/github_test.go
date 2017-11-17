package route

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type GithubWebhookRouteSuite struct {
	sc     *data.MockConnector
	rm     *RouteManager
	prBody []byte
	suite.Suite
}

func (s *GithubWebhookRouteSuite) SetupSuite() {
	ctx := context.Background()
	err := evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings))
	s.NoError(err)
	s.NotNil(evergreen.GetEnvironment().Settings())
	s.NotNil(evergreen.GetEnvironment().Settings().Api)
}

func (s *GithubWebhookRouteSuite) SetupTest() {
	s.rm = getGithubHooksRouteManager("", 2)
	s.sc = &data.MockConnector{MockPatchIntentConnector: data.MockPatchIntentConnector{
		CachedIntents: map[string]patch.Intent{},
	}}

	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))

	s.NoError(err)
	s.Len(s.prBody, 24743)
}

func TestGithubWebhookRouteSuite(t *testing.T) {
	s := new(GithubWebhookRouteSuite)
	suite.Run(t, s)
}

func (s *GithubWebhookRouteSuite) TestAddIntent() {
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)

	s.rm.Methods[0].RequestHandler.(*githubHookApi).event = event
	s.rm.Methods[0].RequestHandler.(*githubHookApi).msgId = "1"

	ctx := context.Background()
	resp, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.Empty(resp.Result)

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 1)
}

func (s *GithubWebhookRouteSuite) TestAddDuplicateIntentFails() {
	s.TestAddIntent()

	ctx := context.Background()
	resp, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.Error(err)
	s.Empty(resp.Result)

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 1)
}

func (s *GithubWebhookRouteSuite) TestAddIntentWithClosedPRHasNoSideEffects() {
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)
	*event.(*github.PullRequestEvent).Action = "closed"

	s.rm.Methods[0].RequestHandler.(*githubHookApi).event = event
	s.rm.Methods[0].RequestHandler.(*githubHookApi).msgId = "1"

	ctx := context.Background()
	resp, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.Empty(resp.Result)

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 0)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidateFailsWithoutSignature() {
	ctx := context.Background()
	req, err := makeRequest("1", s.prBody)
	s.NoError(err)
	req.Header.Del("X-Hub-Signature")

	err = s.rm.Methods[0].RequestHandler.ParseAndValidate(ctx, req)
	s.Error(err)
}

func (s *GithubWebhookRouteSuite) TestParseAndValidate() {
	ctx := context.Background()
	req, err := makeRequest("1", s.prBody)
	s.NoError(err)

	err = s.rm.Methods[0].RequestHandler.ParseAndValidate(ctx, req)
	s.NoError(err)
}

func makeRequest(uid string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest("POST", "http://example.com/rest/v2/hooks/github", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// from genMAC in google/go-github/github/messages.go
	mac := hmac.New(sha256.New, []byte(evergreen.GetEnvironment().Settings().Api.GithubWebhookSecret))
	n, err := mac.Write(body)
	if n != len(body) {
		return nil, errors.Errorf("Body length expected to be %d, but was %d", len(body), n)
	}

	if err != nil {
		return nil, err
	}
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req.Header.Add("Content-type", "application/json")
	req.Header.Add("X-Github-Event", "pull_request")
	req.Header.Add("X-GitHub-Delivery", uid)
	req.Header.Add("X-Hub-Signature", signature)
	return req, nil
}
