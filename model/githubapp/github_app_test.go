package githubapp

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func init() {
	testutil.Setup()
}

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
	_, err := evergreen.GetEnvironment().DB().Collection(GitHubAppCollection).DeleteMany(s.ctx, bson.M{})
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
		AppID:          1234,
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

	installation.Repo = "evergreen"
	installation.AppID = 0
	err = installation.Upsert(s.ctx)
	s.Error(err)
	s.Equal("App ID must not be 0", err.Error())

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

	authFields := &GithubAppAuth{
		AppID: 1234,
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

func (s *installationSuite) TestCreateCachedInstallationToken() {
	installation := GitHubAppInstallation{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		AppID:          1234,
		InstallationID: 5678,
	}
	s.NoError(installation.Upsert(s.ctx))

	const (
		unrestrictedToken = "unrestricted_token"
		restrictedToken   = "restricted_token"
		lifetime          = time.Minute
	)

	// Test without permissions
	ghInstallationTokenCache.put(installation.InstallationID, unrestrictedToken, nil, time.Now())

	authFields := GithubAppAuth{
		AppID: installation.AppID,
	}
	token, err := authFields.CreateCachedInstallationToken(s.ctx, installation.Owner, installation.Repo, lifetime, nil)
	s.Require().NoError(err)
	s.Equal(unrestrictedToken, token, "should return cached token since it is still valid for at least %s", lifetime)

	// Test with permissions
	p := &github.InstallationPermissions{
		Contents: github.String("read"),
		Issues:   github.String("write"),
	}
	opts := &github.InstallationTokenOptions{
		Permissions: p,
	}

	ghInstallationTokenCache.put(installation.InstallationID, restrictedToken, p, time.Now())

	token, err = authFields.CreateCachedInstallationToken(s.ctx, installation.Owner, installation.Repo, lifetime, opts)
	s.Require().NoError(err)
	s.Equal(restrictedToken, token, "should return cached token since it is still valid for at least %s", lifetime)
}
func TestCreateGitHubAppAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	settings := env.Settings()
	settings.AuthConfig.Github = &evergreen.GithubAuthConfig{}
	delete(settings.Expansions, evergreen.GithubAppPrivateKey)

	authFields := CreateGitHubAppAuth(settings)
	assert.Equal(t, "", authFields.Id)

	settings.AuthConfig.Github = &evergreen.GithubAuthConfig{
		AppId: 1234,
	}
	authFields = CreateGitHubAppAuth(settings)
	assert.Nil(t, authFields)

	settings.Expansions[evergreen.GithubAppPrivateKey] = "key"
	authFields = CreateGitHubAppAuth(settings)
	assert.NotNil(t, authFields)
	assert.Equal(t, int64(1234), authFields.AppID)
	assert.Equal(t, []byte("key"), authFields.PrivateKey)
}

func TestCreateCacheID(t *testing.T) {
	testCases := map[string]struct {
		installationID int64
		permissions    *github.InstallationPermissions
		expected       string
	}{
		"NoPermissions": {
			installationID: 1234,
			permissions:    nil,
			expected:       "1234",
		},
		"EmptyPermissions": {
			installationID: 1234,
			permissions:    &github.InstallationPermissions{},
			expected:       "1234",
		},
		"SinglePermission": {
			installationID: 1234,
			permissions: &github.InstallationPermissions{
				Contents: github.String("read"),
			},
			expected: "1234_contents:read",
		},
		"MultiplePermissions": {
			installationID: 1234,
			permissions: &github.InstallationPermissions{
				Contents: github.String("read"),
				Issues:   github.String("write"),
			},
			expected: "1234_contents:read_issues:write",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result, err := createCacheID(tc.installationID, tc.permissions)
			assert.Equal(t, tc.expected, result)
			assert.NoError(t, err)
		})
	}
}
