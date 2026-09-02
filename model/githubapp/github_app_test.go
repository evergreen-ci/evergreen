package githubapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/google/go-github/v70/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
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

// evictCachedTokenAtCleanup drops a cache ID once the test ends.
// ghInstallationTokenCache is process-global, so entries otherwise leak into
// whichever test runs next.
func (s *installationSuite) evictCachedTokenAtCleanup(id string) {
	s.T().Cleanup(func() {
		ghInstallationTokenCache.Delete(s.ctx, id)
	})
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
	id, err := createCacheID(installation.InstallationID, nil, nil)
	s.NoError(err)
	s.Equal("5678", id)
	s.evictCachedTokenAtCleanup(id)
	ghInstallationTokenCache.Put(s.ctx, id, unrestrictedToken, time.Now().Add(lifetime*2))

	authFields := GithubAppAuth{
		AppID: installation.AppID,
	}
	token, err := authFields.CreateCachedInstallationToken(s.ctx, installation.Owner, installation.Repo, lifetime, nil, false)
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

	id, err = createCacheID(installation.InstallationID, p, nil)
	s.NoError(err)
	s.Equal("5678_contents:read_issues:write", id)
	ghInstallationTokenCache.Put(s.ctx, id, restrictedToken, time.Now().Add(lifetime*2))

	token, err = authFields.CreateCachedInstallationToken(s.ctx, installation.Owner, installation.Repo, lifetime, opts, false)
	s.Require().NoError(err)
	s.Equal(restrictedToken, token, "should return cached token since it is still valid for at least %s", lifetime)
}

func (s *installationSuite) TestRefreshCachedInstallationToken() {
	installation := GitHubAppInstallation{
		Owner:          "evergreen-ci",
		Repo:           "evergreen",
		AppID:          1234,
		InstallationID: 5678,
	}
	s.NoError(installation.Upsert(s.ctx))

	const (
		cachedToken = "cached_token"
		lifetime    = time.Minute
	)

	id, err := createCacheID(installation.InstallationID, nil, nil)
	s.Require().NoError(err)
	s.evictCachedTokenAtCleanup(id)
	ghInstallationTokenCache.Put(s.ctx, id, cachedToken, time.Now().Add(lifetime*2))

	// Refreshing skips the cache and mints a new token, which fails here because
	// the app has no private key configured.
	authFields := GithubAppAuth{AppID: installation.AppID}
	_, err = authFields.CreateCachedInstallationToken(s.ctx, installation.Owner, installation.Repo, lifetime, nil, true)
	s.Error(err, "refreshing should not return the still-valid cached token")

	cached, found := ghInstallationTokenCache.Get(s.ctx, id, lifetime)
	s.True(found, "a failed refresh should leave the cached token alone")
	s.Equal(cachedToken, cached)
}

func TestCreateGitHubAppAuth(t *testing.T) {
	ctx := t.Context()

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
		repositories   []string
		expected       string
	}{
		"NoPermissionsOrRepos": {
			installationID: 1234,
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
		"MultipleRepositoriesAreSorted": {
			installationID: 1234,
			repositories:   []string{"bravo", "alpha"},
			expected:       "1234_repos:alpha,bravo",
		},
		"RepositoriesWithPermissions": {
			installationID: 1234,
			repositories:   []string{"myrepo"},
			permissions: &github.InstallationPermissions{
				Contents: github.String("read"),
			},
			expected: "1234_repos:myrepo_contents:read",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result, err := createCacheID(tc.installationID, tc.permissions, tc.repositories)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGithubClientShouldRetry(t *testing.T) {
	makeRequest := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "https://api.github.com/app/installations/1/access_tokens", nil)
	}

	t.Run("BadRequestWithoutOptInDoesNotRetry", func(t *testing.T) {
		retryFn := githubClientShouldRetry(retryConfig{})
		resp := &http.Response{StatusCode: http.StatusBadRequest}
		assert.False(t, retryFn(0, makeRequest(), resp, nil))
	})

	t.Run("BadRequestWithOptInRetries", func(t *testing.T) {
		retryFn := githubClientShouldRetry(retryConfig{retry400: true})
		resp := &http.Response{StatusCode: http.StatusBadRequest}
		assert.True(t, retryFn(0, makeRequest(), resp, nil))
	})

	t.Run("ServerErrorRetriesWithoutOptIn", func(t *testing.T) {
		retryFn := githubClientShouldRetry(retryConfig{})
		resp := &http.Response{StatusCode: http.StatusInternalServerError}
		assert.True(t, retryFn(0, makeRequest(), resp, nil))
	})

	t.Run("SuccessfulResponseDoesNotRetry", func(t *testing.T) {
		retryFn := githubClientShouldRetry(retryConfig{retry400: true})
		resp := &http.Response{StatusCode: http.StatusOK}
		assert.False(t, retryFn(0, makeRequest(), resp, nil))
	})
}
