package route

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
)

func init() {
	ctx := context.Background()
	err := evergreen.GetEnvironment().Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings))
	if err != nil {
		panic("Failed to Configure environment")
	}
}

type PatchIntentConnectorSuite struct {
	sc     *data.MockConnector
	rm     *RouteManager
	prBody []byte
	suite.Suite
}

func (s *PatchIntentConnectorSuite) SetupTest() {
	s.rm = getGithubHooksRouteManager("", 2)
	s.sc = &data.MockConnector{MockPatchIntentConnector: data.MockPatchIntentConnector{
		CachedIntents: []patch.Intent{},
	}}

	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))

	s.NoError(err)
	s.Len(s.prBody, 24743)
}

func TestPatchIntentConnectorSuite(t *testing.T) {
	s := new(PatchIntentConnectorSuite)
	suite.Run(t, s)
}

func (s *PatchIntentConnectorSuite) TestAddIntent() {
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)

	s.rm.Methods[0].RequestHandler.(*githubHookApi).event = event
	s.rm.Methods[0].RequestHandler.(*githubHookApi).msgId = "1"

	ctx := context.Background()
	resp, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.Len(resp.Result, 0)

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 1)
}

func (s *PatchIntentConnectorSuite) TestAddIntentWithClosedPRHasNoSideEffects() {
	event, err := github.ParseWebHook("pull_request", s.prBody)
	s.NotNil(event)
	s.NoError(err)
	*event.(*github.PullRequestEvent).Action = "closed"

	s.rm.Methods[0].RequestHandler.(*githubHookApi).event = event
	s.rm.Methods[0].RequestHandler.(*githubHookApi).msgId = "1"

	ctx := context.Background()
	resp, err := s.rm.Methods[0].Execute(ctx, s.sc)
	s.NoError(err)
	s.Len(resp.Result, 0)

	s.Len(s.sc.MockPatchIntentConnector.CachedIntents, 0)
}
