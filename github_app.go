package evergreen

import (
	"context"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/evergreen-ci/utility"
	"github.com/golang-jwt/jwt"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	ownerKey          = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey           = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	installationIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "InstallationID")
)

var (
	gitHubAppNotInstalledError = errors.New("GitHub app is not installed")
)

type GitHubAppInstallation struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`

	// InstallationID is the GitHub app's installation ID for the owner/repo.
	InstallationID int64 `bson:"installation_id"`
}

type githubAppAuth struct {
	appId      int64
	privateKey []byte
}

// getGithubAppAuth returns the app id and app private key if they exist.
func getGithubAppAuth(s *Settings) *githubAppAuth {
	if s.AuthConfig.Github == nil || s.AuthConfig.Github.AppId == 0 {
		return nil
	}

	key := s.Expansions[githubAppPrivateKey]
	if key == "" {
		return nil
	}

	return &githubAppAuth{
		appId:      s.AuthConfig.Github.AppId,
		privateKey: []byte(key),
	}
}

// HasGitHubApp returns true if the GitHub app is installed on given owner/repo.
func (s *Settings) HasGitHubApp(ctx context.Context, owner, repo string) (bool, error) {
	authFields := getGithubAppAuth(s)
	if authFields == nil {
		return false, errors.New("GitHub app is not configured in admin settings")
	}

	installationID, err := getInstallationID(ctx, authFields, owner, repo)
	if err != nil {
		return false, errors.Wrapf(err, "getting installation id for '%s/%s'", owner, repo)
	}

	return installationID != 0, nil
}

// CreateInstallationTokenWithDefaultOwnerRepo returns an installation token when we do not care about
// the owner/repo that we are calling the GitHub function with (i.e. checking rate limit).
// It will use the default owner/repo specified in the admin settings and error if it's not set.
func (s *Settings) CreateInstallationTokenWithDefaultOwnerRepo(ctx context.Context, opts *github.InstallationTokenOptions) (string, error) {
	if s.AuthConfig.Github == nil || s.AuthConfig.Github.DefaultOwner == "" || s.AuthConfig.Github.DefaultRepo == "" {
		// TODO EVG-19966: Return error here
		grip.Debug(message.Fields{
			"message": "no default owner/repo",
			"ticket":  "EVG-19966",
		})
		return "", nil
	}
	return s.CreateInstallationToken(ctx, s.AuthConfig.Github.DefaultOwner, s.AuthConfig.Github.DefaultRepo, opts)
}

// CreateInstallationToken uses the owner/repo information to request an github app installation id
// and uses that id to create an installation token.
func (s *Settings) CreateInstallationToken(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	authFields := getGithubAppAuth(s)
	if authFields == nil {
		return "", errors.New("GitHub app is not configured in admin settings")
	}

	installationID, err := getInstallationID(ctx, authFields, owner, repo)
	if err != nil {
		return "", errors.Wrapf(err, "getting installation id for '%s/%s'", owner, repo)
	}

	token, err := createInstallationToken(ctx, authFields, installationID, opts)
	if err != nil {
		return "", errors.Wrapf(err, "creating installation token for '%s/%s'", owner, repo)
	}

	return token, nil
}

func getInstallationID(ctx context.Context, authFields *githubAppAuth, owner, repo string) (int64, error) {
	cachedID, err := getInstallationIDFromCache(ctx, owner, repo)
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
	}
	if err := cachedInstallation.Upsert(ctx); err != nil {
		return 0, errors.Wrapf(err, "caching installation id for '%s/%s'", owner, repo)
	}

	return installationID, nil

}

func byOwnerRepo(owner, repo string) bson.M {
	q := bson.M{
		ownerKey: owner,
		repoKey:  repo,
	}
	return q
}

func validateOwnerRepo(owner, repo string) error {
	if len(owner) == 0 || len(repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}
	return nil
}

// Upsert updates the installation information in the database.
func (h *GitHubAppInstallation) Upsert(ctx context.Context) error {
	if err := validateOwnerRepo(h.Owner, h.Repo); err != nil {
		return err
	}

	_, err := GetEnvironment().DB().Collection(GitHubAppCollection).UpdateOne(
		ctx,
		byOwnerRepo(h.Owner, h.Repo),
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
func getInstallationIDFromCache(ctx context.Context, owner, repo string) (int64, error) {
	if err := validateOwnerRepo(owner, repo); err != nil {
		return 0, err
	}

	installation := &GitHubAppInstallation{}
	res := GetEnvironment().DB().Collection(GitHubAppCollection).FindOne(ctx, byOwnerRepo(owner, repo))
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

// getGitHubClientForAuth returns a GitHub client with the GitHub app's private key.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
func getGitHubClientForAuth(authFields *githubAppAuth) (*github.Client, error) {
	retryConf := utility.NewDefaultHTTPRetryConf()
	retryConf.MaxDelay = GitHubRetryMaxDelay
	retryConf.BaseDelay = GitHubRetryMinDelay
	retryConf.MaxRetries = GitHubMaxRetries

	httpClient := utility.GetHTTPRetryableClient(retryConf)

	key, err := jwt.ParseRSAPrivateKeyFromPEM(authFields.privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}

	itr := ghinstallation.NewAppsTransportFromPrivateKey(httpClient.Transport, authFields.appId, key)
	httpClient.Transport = itr
	client := github.NewClient(httpClient)
	return client, nil
}

// getInstallationIDFromGitHub returns an installation ID from GitHub given an owner and a repo.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
func getInstallationIDFromGitHub(ctx context.Context, authFields *githubAppAuth, owner, repo string) (int64, error) {
	client, err := getGitHubClientForAuth(authFields)
	if err != nil {
		return 0, errors.Wrap(err, "getting GitHub client to get the installation ID")
	}

	installation, resp, err := client.Apps.FindRepositoryInstallation(ctx, owner, repo)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
		}
		if resp.StatusCode == http.StatusNotFound {
			return 0, errors.Wrapf(gitHubAppNotInstalledError, "installation id for '%s/%s' not found", owner, repo)
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error finding installation id",
			"owner":   owner,
			"repo":    repo,
			"appId":   authFields.appId,
			"ticket":  "EVG-19966",
		}))
		return 0, errors.Wrapf(err, "finding installation id for '%s/%s'", owner, repo)
	}
	if installation == nil {
		return 0, errors.Errorf("Installation id for '%s/%s' not found", owner, repo)
	}

	return installation.GetID(), nil
}

// createInstallationToken returns an installation token from GitHub given an installation ID.
// This function cannot be moved to thirdparty because it is needed to set up the environment.
func createInstallationToken(ctx context.Context, authFields *githubAppAuth, installationID int64, opts *github.InstallationTokenOptions) (string, error) {
	client, err := getGitHubClientForAuth(authFields)
	if err != nil {
		return "", errors.Wrap(err, "getting GitHub client for token creation")
	}

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
