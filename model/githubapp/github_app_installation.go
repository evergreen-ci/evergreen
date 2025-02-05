package githubapp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/golang-jwt/jwt"
	"github.com/google/go-github/v52/github"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	GitHubMaxRetries    = 3
	GitHubRetryMinDelay = time.Second
	GitHubRetryMaxDelay = 10 * time.Second
)

var (
	gitHubAppNotInstalledError = errors.New("GitHub app is not installed")
)

// GitHubAppInstallation holds information about a GitHub app, notably its
// installation ID. This does not contain actual GitHub app credentials.
type GitHubAppInstallation struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`

	// InstallationID is the GitHub app's installation ID for the owner/repo.
	InstallationID int64 `bson:"installation_id"`

	// AppID is the id of the GitHub app that the installation ID is associated with
	AppID int64 `bson:"app_id"`
}

func getInstallationID(ctx context.Context, authFields *GithubAppAuth, owner, repo string) (int64, error) {
	cachedID, err := getInstallationIDFromCache(ctx, authFields.AppID, owner, repo)
	if err != nil {
		return 0, errors.Wrapf(err, "getting cached installation id for '%s/%s'", owner, repo)
	}
	if cachedID != 0 {
		return cachedID, nil
	}

	installationID, err := getInstallationIDFromGitHub(ctx, authFields, owner, repo)
	if err != nil {
		return 0, errors.Wrapf(err, "getting installation id for '%s/%s'", owner, repo)
	}

	cachedInstallation := GitHubAppInstallation{
		Owner:          owner,
		Repo:           repo,
		InstallationID: installationID,
		AppID:          authFields.AppID,
	}

	if err := cachedInstallation.Upsert(ctx); err != nil {
		return 0, errors.Wrapf(err, "caching installation id for '%s/%s'", owner, repo)
	}

	return installationID, nil

}

// Upsert updates the installation information in the database.
func (h *GitHubAppInstallation) Upsert(ctx context.Context) error {
	if err := validateOwnerRepo(h.AppID, h.Owner, h.Repo); err != nil {
		return err
	}

	_, err := evergreen.GetEnvironment().DB().Collection(GitHubAppCollection).UpdateOne(
		ctx,
		byAppOwnerRepo(h.AppID, h.Owner, h.Repo),
		bson.M{
			"$set": h,
		},
		options.Update().SetUpsert(true),
	)
	return err
}

// GitHubClient adds a Close method to the GitHub client that
// puts the underlying HTTP client back into the pool.
type GitHubClient struct {
	*github.Client
}

// Close puts the underlying HTTP client back into the pool.
func (g *GitHubClient) Close() {
	if g == nil {
		return
	}
	if client := g.Client.Client(); client != nil {
		utility.PutHTTPClient(client)
	}
}

// getGitHubClientForAuth returns a GitHub client with the GitHub app's private key.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
// Couple this with a defered call with Close() to clean up the client.
func getGitHubClientForAuth(authFields *GithubAppAuth) (*GitHubClient, error) {
	// retryConf := utility.NewDefaultHTTPRetryConf()
	// retryConf.MaxDelay = GitHubRetryMaxDelay
	// retryConf.BaseDelay = GitHubRetryMinDelay
	// retryConf.MaxRetries = GitHubMaxRetries

	key, err := jwt.ParseRSAPrivateKeyFromPEM(authFields.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}

	// kim: TODO: unsure if this is the same. Logically, it should be the same
	// as before, but it wraps the GH auth transport in the rehttp transport,
	// which means we can add custom retry logic on top of GH auth'd requests.
	// httpClient := utility.GetHTTPClient(retryConf)
	// kim: TODO: see if this can be wrapped in a rehttp.NewTransport, which
	// accepts a RoundTripper. The transport returned from this already fulfills
	// the RoundTripper interface.
	// itr := ghinstallation.NewAppsTransportFromPrivateKey(httpClient.Transport, authFields.AppID, key)
	// httpClient.Transport = itr
	itr := ghinstallation.NewAppsTransportFromPrivateKey(utility.DefaultTransport(), authFields.AppID, key)
	httpClient := utility.GetCustomHTTPRetryableClientWithTransport(itr, githubClientShouldRetry(), utility.RetryHTTPDelay(utility.RetryOptions{
		MinDelay:    GitHubRetryMinDelay,
		MaxDelay:    GitHubRetryMaxDelay,
		MaxAttempts: GitHubMaxRetries + 1,
	}))

	client := github.NewClient(httpClient)
	wrappedClient := GitHubClient{Client: client}
	return &wrappedClient, nil
}

func githubClientShouldRetry() utility.HTTPRetryFunction {
	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		// kim: TODO: add custom logic for retrying on GitHub app errors. May
		// unfortunately require reading the body depending on how the response
		// is returned.
		// Found errors:
		// * EOF
		// * read: connection reset by peer
		// * 504 We couldn't respond to your request in time. Sorry about that. Please try resubmitting your request and contact us if the problem persists.
		// * 403 You have exceeded a secondary rate limit. Please wait a few minutes before you try again. If you reach out to GitHub Support for help, please include the request ID <REQUEST_ID>
		// Note: may have to read the response body. Then either retry or
		// restore the response body and pass it along to the caller. Seems like
		// most code here doesn't use the response body anyways, so it's
		// probably fine to restore it.
		return false
	}
}

// getInstallationIDFromGitHub returns an installation ID from GitHub given an owner and a repo.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
func getInstallationIDFromGitHub(ctx context.Context, authFields *GithubAppAuth, owner, repo string) (int64, error) {
	client, err := getGitHubClientForAuth(authFields)
	if err != nil {
		return 0, errors.Wrap(err, "getting GitHub client to get the installation ID")
	}
	defer client.Close()

	installation, resp, err := client.Apps.FindRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
		} else {
			return 0, errors.Wrapf(err, "finding installation id for '%s/%s'", owner, repo)
		}
		if resp.StatusCode == http.StatusNotFound {
			return 0, errors.Wrapf(gitHubAppNotInstalledError, "installation id for '%s/%s' not found", owner, repo)
		}
		return 0, errors.Wrapf(err, "finding installation id for '%s/%s'", owner, repo)
	}
	if installation == nil {
		return 0, errors.Errorf("Installation id for '%s/%s' not found", owner, repo)
	}

	return installation.GetID(), nil
}

// cachedInstallationToken represents a GitHub installation token that's
// cached in memory.
type cachedInstallationToken struct {
	installationToken string
	expiresAt         time.Time
}

func (c *cachedInstallationToken) isExpired(lifetime time.Duration) bool {
	return time.Until(c.expiresAt) < lifetime
}

// installationTokenCache is a concurrency-safe cache mapping the installation
// ID to the cached GitHub installation token for it.
type installationTokenCache struct {
	cache map[int64]cachedInstallationToken
	mu    sync.RWMutex
}

// ghInstallationTokenCache is the in-memory instance of the cache for GitHub
// installation tokens.
var ghInstallationTokenCache = installationTokenCache{
	cache: make(map[int64]cachedInstallationToken),
	mu:    sync.RWMutex{},
}

// get gets an installation token from the cache by its installation ID. It will
// not return a token if the token will expire before the requested lifetime.
func (c *installationTokenCache) get(installationID int64, lifetime time.Duration) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cachedToken, ok := c.cache[installationID]
	if !ok {
		return ""
	}
	if cachedToken.isExpired(lifetime) {
		return ""
	}

	return cachedToken.installationToken
}

// MaxInstallationTokenLifetime is the maximum amount of time that an
// installation token can be used before it expires.
const MaxInstallationTokenLifetime = time.Hour

func (c *installationTokenCache) put(installationID int64, installationToken string, createdAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[installationID] = cachedInstallationToken{
		installationToken: installationToken,
		expiresAt:         createdAt.Add(MaxInstallationTokenLifetime),
	}
}
