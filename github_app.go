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
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	GitHubAppCollection = "github_hooks"
)

type GitHubAppInstallation struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`

	// InstallationID is the GitHub app's installation ID for the owner/repo.
	InstallationID int64 `bson:"installation_id"`
}

//nolint:megacheck,unused
var (
	ownerKey          = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey           = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	installationIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "InstallationID")

	gitHubAppNotInstalledError = errors.New("GitHub app is not installed")
)

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

// getInstallationID returns the installation ID for GitHub app if it's installed for the given owner/repo.
func getInstallationID(ctx context.Context, owner, repo string) (int64, error) {
	if err := validateOwnerRepo(owner, repo); err != nil {
		return 0, err
	}

	installation := &GitHubAppInstallation{}
	res := GetEnvironment().DB().Collection(GitHubAppCollection).FindOne(ctx, byOwnerRepo(owner, repo))
	if err := res.Err(); err != nil {
		return 0, errors.Wrapf(err, "finding installation ID for '%s/%s", owner, repo)
	}
	if err := res.Decode(&installation); err != nil {
		return 0, errors.Wrapf(err, "decoding installation ID for '%s/%s", owner, repo)
	}

	return installation.InstallationID, nil
}

type githubAppAuth struct {
	AppId      int64
	privateKey []byte
}

// getGithubAppAuth returns app id and app private key if it exists.
func (s *Settings) getGithubAppAuth() *githubAppAuth {
	if s.AuthConfig.Github == nil || s.AuthConfig.Github.AppId == 0 {
		return nil
	}

	key := s.Expansions[githubAppPrivateKey]
	if key == "" {
		return nil
	}

	return &githubAppAuth{
		AppId:      s.AuthConfig.Github.AppId,
		privateKey: []byte(key),
	}
}

// CreateInstallationToken uses the owner/repo information to request an github app installation id
// and uses that id to create an installation token.
func (s *Settings) CreateInstallationToken(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	const (
		maxDelay   = 10 * time.Second
		minDelay   = time.Second
		maxRetries = 5
	)

	if owner == "" || repo == "" {
		return "", errors.New("no owner/repo specified to create installation token")
	}
	authFields := s.getGithubAppAuth()
	if authFields == nil {
		return "", errors.New("GitHub app is not configured in admin settings")
	}

	retryConf := utility.NewDefaultHTTPRetryConf()
	retryConf.MaxDelay = maxDelay
	retryConf.BaseDelay = minDelay
	retryConf.MaxRetries = maxRetries

	httpClient := utility.GetHTTPRetryableClient(retryConf)
	defer utility.PutHTTPClient(httpClient)

	key, err := jwt.ParseRSAPrivateKeyFromPEM(authFields.privateKey)
	if err != nil {
		return "", errors.Wrap(err, "parsing private key")
	}

	itr := ghinstallation.NewAppsTransportFromPrivateKey(httpClient.Transport, authFields.AppId, key)
	httpClient.Transport = itr
	client := github.NewClient(httpClient)

	installationID, err := getInstallationID(ctx, owner, repo)
	if err != nil {
		return "", errors.Wrapf(err, "getting cached installation id for '%s/%s'", owner, repo)
	}
	if installationID == 0 {
		installationID, resp, err := client.Apps.FindRepositoryInstallation(ctx, owner, repo)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				return "", errors.Wrapf(gitHubAppNotInstalledError, "installation id for '%s/%s' not found", owner, repo)
			}
			grip.Debug(message.WrapError(err, message.Fields{
				"message": "error finding installation id",
				"owner":   owner,
				"repo":    repo,
				"appId":   authFields.AppId,
				"ticket":  "EVG-19966",
			}))
			return "", errors.Wrapf(err, "finding installation id for '%s/%s'", owner, repo)
		}
		if installationID == nil {
			return "", errors.Errorf("Installation id for '%s/%s' not found", owner, repo)
		}

		// Cache the installation ID for owner/repo.
		installation := GitHubAppInstallation{
			Owner:          owner,
			Repo:           repo,
			InstallationID: installationID.GetID(),
		}
		if err := installation.Upsert(ctx); err != nil {
			return "", errors.Wrapf(err, "saving installation id for '%s/%s'", owner, repo)
		}
	}

	token, _, err := client.Apps.CreateInstallationToken(ctx, installationID, opts)
	if err != nil || token == nil {
		return "", errors.Wrapf(err, "creating installation token for installation id: %d", installationID)
	}
	return token.GetToken(), nil
}

// HasGitHubApp returns true if the GitHub app is installed on given owner/repo.
// Only returns an error if app is installed.
func (s *Settings) HasGitHubApp(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (bool, error) {
	// Check cache for installation ID first.
	installationID, err := getInstallationID(ctx, owner, repo)
	if err != nil {
		return false, errors.Wrapf(err, "getting cached installation id for '%s/%s'", owner, repo)
	}
	if installationID != 0 {
		return true, nil
	}

	token, err := s.CreateInstallationToken(ctx, owner, repo, opts)
	if err != nil {
		if errors.Is(err, gitHubAppNotInstalledError) {
			return false, nil
		}
		return false, errors.Wrap(err, "verifying GitHub app installation")
	}
	return token != "", nil
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
