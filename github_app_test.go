package evergreen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type installationSuite struct {
	ctx    context.Context
	cancel context.CancelFunc

	suite.Suite
}

func TestGithubInstallationSuite(t *testing.T) {
	suite.Run(t, new(installationSuite))
}

func (s *installationSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	_, err := GetEnvironment().DB().Collection(GitHubAppCollection).DeleteMany(s.ctx, bson.M{})
	s.NoError(err)
}

func (s *installationSuite) TearDownTest() {
	s.cancel()
}

func (s *installationSuite) TestUpsert() {
	installation := GitHubAppInstallation{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		InstallationID: 0,
	}

	s.NoError(installation.Upsert(s.ctx))

	installation.Owner = ""
	err := installation.Upsert(s.ctx)
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	installation.Owner = "evergreen-ci"
	installation.Repo = ""
	err = installation.Upsert(s.ctx)
	s.Error(err)
	s.Equal("Owner and repository must not be empty strings", err.Error())

	installationWithInstallationAndAppID := GitHubAppInstallation{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		AppID:          1234,
		InstallationID: 5678,
	}
	s.NoError(installationWithInstallationAndAppID.Upsert(s.ctx))
}

func (s *installationSuite) TestGetInstallationID() {
	installation := GitHubAppInstallation{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		AppID:          1234,
		InstallationID: 5678,
	}

	s.NoError(installation.Upsert(s.ctx))

	authFields := &githubAppAuth{
		appId: 1234,
	}

	id, err := getInstallationID(s.ctx, authFields, "evergreen-ci", "evergreen")
	s.NoError(err)
	s.Equal(installation.InstallationID, id)

	_, err = getInstallationID(s.ctx, authFields, "evergreen-ci", "")
	s.Error(err)

	_, err = getInstallationID(s.ctx, authFields, "", "evergreen")
	s.Error(err)

	_, err = getInstallationID(s.ctx, authFields, "", "")
	s.Error(err)
}
