package evergreen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type hookSuite struct {
	ctx    context.Context
	cancel context.CancelFunc

	suite.Suite
}

func TestGithubHookSuite(t *testing.T) {
	suite.Run(t, new(hookSuite))
}

func (s *hookSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	_, err := GetEnvironment().DB().Collection(GitHubHooksCollection).DeleteMany(s.ctx, bson.M{})
	s.NoError(err)
}

func (s *hookSuite) TearDownTest() {
	s.cancel()
}

func (s *hookSuite) TestUpsert() {
	hook := GitHubHook{
		HookID:         1,
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		InstallationID: 0,
	}

	s.NoError(hook.Upsert(s.ctx))

	hook.Owner = ""
	err := hook.Upsert(s.ctx)
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	hook.Owner = "evergreen-ci"
	hook.Repo = ""
	err = hook.Upsert(s.ctx)
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	hookWithInstallationID := GitHubHook{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		InstallationID: 1234,
	}
	s.NoError(hookWithInstallationID.Upsert(s.ctx))
}

func (s *hookSuite) TestGetInstallationID() {
	hook := GitHubHook{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		InstallationID: 1234,
	}

	s.NoError(hook.Upsert(s.ctx))

	id, err := getInstallationID(s.ctx, "evergreen-ci", "evergreen")
	s.NoError(err)
	s.Equal(hook.InstallationID, id)

	_, err = getInstallationID(s.ctx, "evergreen-ci", "")
	s.Error(err)

	_, err = getInstallationID(s.ctx, "", "evergreen")
	s.Error(err)

	_, err = getInstallationID(s.ctx, "", "")
	s.Error(err)

	hook.InstallationID = 0
	s.NoError(hook.Upsert(s.ctx))

	id, err = getInstallationID(s.ctx, "evergreen-ci", "evergreen")
	s.NoError(err)
	s.Equal(hook.InstallationID, id)
}
