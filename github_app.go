package evergreen

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/evergreen-ci/utility"
	"github.com/golang-jwt/jwt"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	GitHubAppCollection = "github_hooks"

	GitHubMaxRetries    = 3
	GitHubRetryMinDelay = time.Second
	GitHubRetryMaxDelay = 10 * time.Second
)

//nolint:megacheck,unused
var (
	ownerKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey  = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	appIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "AppID")
)

var (
	gitHubAppNotInstalledError = errors.New("GitHub app is not installed")
)

type GitHubAppInstallation struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`

	// InstallationID is the GitHub app's installation ID for the owner/repo.
	InstallationID int64 `bson:"installation_id"`

	// AppID is the id of the GitHub app that the installation ID is associated with
	AppID int64 `bson:"app_id"`
}

// GithubAppAuth hold the appId and privateKey for the github app associated with the project.
// It will not be stored along with the project settings, instead it is fetched only when needed
// Sometimes this struct is used as a way to pass around AppId and PrivateKey for Evergreen's
// github app, in which the Id is set to empty.
type GithubAppAuth struct {
	// Should match the identifier of the project it refers to
	Id string `bson:"_id" json:"_id"`

	AppID      int64  `bson:"app_id" json:"app_id"`
	PrivateKey []byte `bson:"private_key" json:"private_key"`
}

// CreateGitHubAppAuth returns the app id and app private key if they exist.
// If the either are not set, it will return nil.
func (s *Settings) CreateGitHubAppAuth() *GithubAppAuth {
	if s.AuthConfig.Github == nil || s.AuthConfig.Github.AppId == 0 {
		return nil
	}

	key := s.Expansions[GithubAppPrivateKey]
	if key == "" {
		return nil
	}

	return &GithubAppAuth{
		AppID:      s.AuthConfig.Github.AppId,
		PrivateKey: []byte(key),
	}
}

// IsGithubAppInstalledOnRepo returns true if the GitHub app is installed on given owner/repo.
func (g *GithubAppAuth) IsGithubAppInstalledOnRepo(ctx context.Context, owner, repo string) (bool, error) {
	if g == nil || g.AppID == 0 || len(g.PrivateKey) == 0 {
		return false, errors.New("no github app auth provided")
	}

	installationID, err := getInstallationID(ctx, g, owner, repo)
	if err != nil {
		return false, errors.Wrapf(err, "getting installation id for '%s/%s'", owner, repo)
	}

	return installationID != 0, nil
}

// CreateInstallationTokenWithDefaultOwnerRepo returns an installation token when we do not care about
// the owner/repo that we are calling the GitHub function with (i.e. checking rate limit).
// It will use the default owner/repo specified in the admin settings and error if it's not set.
func (s *Settings) CreateInstallationTokenWithDefaultOwnerRepo(ctx context.Context, lifetime time.Duration, opts *github.InstallationTokenOptions) (string, error) {
	if s.AuthConfig.Github == nil || s.AuthConfig.Github.DefaultOwner == "" || s.AuthConfig.Github.DefaultRepo == "" {
		return "", errors.Errorf("missing GitHub app configuration needed to create installation tokens")
	}
	return s.CreateGitHubAppAuth().CreateInstallationToken(ctx, s.AuthConfig.Github.DefaultOwner, s.AuthConfig.Github.DefaultRepo, lifetime, opts)
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

// installationTokenCache is a cache mapping the installation ID to the cached
// GitHub installation token for it.
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

// maxInstallationTokenLifetime is the maximum amount of time that an
// installation token can be used before it expires.
const maxInstallationTokenLifetime = time.Hour

func (c *installationTokenCache) put(installationID int64, installationToken string, createdAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[installationID] = cachedInstallationToken{
		installationToken: installationToken,
		expiresAt:         createdAt.Add(maxInstallationTokenLifetime),
	}
}

// CreateInstallationToken uses the owner/repo information to request an github app installation id
// and uses that id to create an installation token.
// If possible, it will try to use an existing installation token for the app
// from the cache, unless that cached token will expire before the requested
// lifetime. For example, if requesting a token that should be valid for the
// next 30 minutes, this method can return a cached token that is still valid
// for 45 minutes. However, if the cached token will expire in 5 minutes, it
// will provide a freshly-generated token.
func (g *GithubAppAuth) CreateInstallationToken(ctx context.Context, owner, repo string, lifetime time.Duration, opts *github.InstallationTokenOptions) (string, error) {
	if lifetime >= maxInstallationTokenLifetime {
		lifetime = maxInstallationTokenLifetime
	}

	if g == nil {
		return "", errors.New("GitHub app is not configured in admin settings")
	}

	installationID, err := getInstallationID(ctx, g, owner, repo)
	if err != nil {
		return "", errors.Wrapf(err, "getting installation id for '%s/%s'", owner, repo)
	}

	token, err := g.getInstallationTokenForID(ctx, installationID, lifetime, opts)
	if err != nil {
		return "", errors.Wrapf(err, "getting installation token for '%s/%s'", owner, repo)
	}

	return token, nil
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

func (g *GithubAppAuth) getInstallationTokenForID(ctx context.Context, installationID int64, lifetime time.Duration, opts *github.InstallationTokenOptions) (string, error) {
	if cachedToken := ghInstallationTokenCache.get(installationID, lifetime); cachedToken != "" {
		return cachedToken, nil
	}

	createdAt := time.Now()
	token, err := g.createInstallationTokenForID(ctx, installationID, opts)
	if err != nil {
		return "", errors.Wrap(err, "creating installation token")
	}

	ghInstallationTokenCache.put(installationID, token, createdAt)

	return token, nil
}

func byAppOwnerRepo(appId int64, owner, repo string) bson.M {
	q := bson.M{
		ownerKey: owner,
		repoKey:  repo,
		appIDKey: appId,
	}
	return q
}

func validateOwnerRepo(appId int64, owner, repo string) error {
	if appId == 0 {
		return errors.New("App ID must not be 0")
	}

	if len(owner) == 0 || len(repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}
	return nil
}

// Upsert updates the installation information in the database.
func (h *GitHubAppInstallation) Upsert(ctx context.Context) error {
	if err := validateOwnerRepo(h.AppID, h.Owner, h.Repo); err != nil {
		return err
	}

	_, err := GetEnvironment().DB().Collection(GitHubAppCollection).UpdateOne(
		ctx,
		byAppOwnerRepo(h.AppID, h.Owner, h.Repo),
		bson.M{
			"$set": h,
		},
		&options.UpdateOptions{
			Upsert: utility.TruePtr(),
		},
	)
	return err
}

// getInstallationID returns the cached installation ID for GitHub app from the database.
func getInstallationIDFromCache(ctx context.Context, app int64, owner, repo string) (int64, error) {
	if err := validateOwnerRepo(app, owner, repo); err != nil {
		return 0, err
	}

	installation := &GitHubAppInstallation{}
	res := GetEnvironment().DB().Collection(GitHubAppCollection).FindOne(ctx, byAppOwnerRepo(app, owner, repo))
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return 0, nil
		}
		return 0, errors.Wrapf(err, "finding cached installation ID for '%s/%s", owner, repo)
	}
	if err := res.Decode(&installation); err != nil {
		return 0, errors.Wrapf(err, "decoding installation ID for '%s/%s", owner, repo)
	}

	return installation.InstallationID, nil
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
	retryConf := utility.NewDefaultHTTPRetryConf()
	retryConf.MaxDelay = GitHubRetryMaxDelay
	retryConf.BaseDelay = GitHubRetryMinDelay
	retryConf.MaxRetries = GitHubMaxRetries

	key, err := jwt.ParseRSAPrivateKeyFromPEM(authFields.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}

	httpClient := utility.GetHTTPRetryableClient(retryConf)
	itr := ghinstallation.NewAppsTransportFromPrivateKey(httpClient.Transport, authFields.AppID, key)
	httpClient.Transport = itr
	client := github.NewClient(httpClient)
	wrappedClient := GitHubClient{Client: client}
	return &wrappedClient, nil
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

// createInstallationTokenForID returns an installation token from GitHub given an installation ID.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
func (g *GithubAppAuth) createInstallationTokenForID(ctx context.Context, installationID int64, opts *github.InstallationTokenOptions) (string, error) {
	client, err := getGitHubClientForAuth(g)
	if err != nil {
		return "", errors.Wrap(err, "getting GitHub client for token creation")
	}
	defer client.Close()

	token, resp, err := client.Apps.CreateInstallationToken(ctx, installationID, opts)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", errors.Wrapf(err, "creating installation token for installation id: '%d'", installationID)
	}
	if token == nil {
		return "", errors.Errorf("Installation token for installation 'id': %d not found", installationID)
	}
	return token.GetToken(), nil
}

// RedactPrivateKey redacts the GitHub app's private key so that it's not exposed via the UI or GraphQL.
func (g *GithubAppAuth) RedactPrivateKey() *GithubAppAuth {
	g.PrivateKey = []byte(RedactedValue)
	return g
}
