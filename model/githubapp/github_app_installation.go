package githubapp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/evergreen-ci/utility/ttlcache"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
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
	key, err := jwt.ParseRSAPrivateKeyFromPEM(authFields.PrivateKey)
	if err != nil {
		return nil, errors.Wrap(err, "parsing private key")
	}

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
	defaultRetryableStatuses := utility.NewDefaultHTTPRetryConf().Statuses
	// The GitHub API returns 403 Forbidden when a secondary rate limit is
	// exceeded. This should ideally be covered already by checking for
	// github.AbuseRateLimitError, but we don't fully trust that the check is
	// comprehensive because GitHub doesn't document its error responses for
	// secondary rate limits.
	defaultRetryableStatuses = append(defaultRetryableStatuses, http.StatusForbidden)

	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		const op = "githubClientShouldRetry"
		_, span := tracer.Start(req.Context(), op)
		defer span.End()

		span.SetAttributes(attribute.Int(githubAppAttemptAttribute, index))
		span.SetAttributes(attribute.String(githubAppURLAttribute, req.URL.String()))
		span.SetAttributes(attribute.String(githubAppMethodAttribute, req.Method))

		makeLogMsg := func(extraFields map[string]any) message.Fields {
			msg := message.Fields{
				"url":     req.URL.String(),
				"method":  req.Method,
				"attempt": index,
				"op":      op,
			}
			for k, v := range extraFields {
				msg[k] = v
			}
			return msg
		}

		if err != nil {
			span.SetAttributes(attribute.String(githubAppErrorAttribute, err.Error()))
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return true
			}
			if utility.IsTemporaryError(err) {
				return true
			}

			if strings.Contains(err.Error(), "connection reset by peer") {
				// This has happened in the past when GitHub was having an
				// outage, so it's worth retrying.
				return true
			}

			if errors.Is(err, &github.AbuseRateLimitError{}) {
				// go-github documentation says it will return
				// github.AbuseRateLimitError if it detects a secondary rate
				// limit is exceeded.
				return true
			}

			grip.Error(message.WrapError(err, makeLogMsg(map[string]any{
				"message": "GitHub endpoint encountered unretryable error",
			})))

			return false
		}

		if resp == nil {
			errMsg := "GitHub app endpoint returned nil response"
			span.SetAttributes(attribute.String(githubAppErrorAttribute, errMsg))
			grip.Error(message.WrapError(err, makeLogMsg(map[string]any{
				"message": errMsg,
			})))
			return true
		}

		span.SetAttributes(attribute.Int(githubAppStatusCodeAttribute, resp.StatusCode))

		for _, statusCode := range defaultRetryableStatuses {
			if resp.StatusCode == statusCode {
				return true
			}
		}

		grip.ErrorWhen(resp.StatusCode >= http.StatusBadRequest, makeLogMsg(map[string]any{
			"message":     "GitHub app endpoint returned response but is not retryable",
			"status_code": resp.StatusCode,
		}))

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

// ghInstallationTokenCache is the in-memory instance of the cache for GitHub
// installation tokens.
var ghInstallationTokenCache = ttlcache.WithOtel(ttlcache.NewInMemory[string](), "github-app-installation-token")

// MaxInstallationTokenLifetime is the maximum amount of time that an
// installation token can be used before it expires.
const MaxInstallationTokenLifetime = time.Hour

// createCacheID creates an ID based on the installation ID and the token's permissions.
// This allows us to put and get installation tokens from the cache based on the installation ID
// and the permissions that the token is scoped to.
// The format of the ID is: "<installationID>_<permissionKey:permissionValue>_<permissionKey:permissionValue>...".
func createCacheID(installationID int64, permissions *github.InstallationPermissions) (string, error) {
	id := fmt.Sprint(installationID)
	if permissions == nil {
		return id, nil
	}
	permissionsStructVal := reflect.ValueOf(permissions).Elem()
	var permissionPairs []string

	// Iterate through the permissions struct and look for fields that are pointers to strings.
	// If the field is not nil, add the field name and its value to the permissionPairs array to
	// be concatenated into the cache ID.
	for i := 0; i < permissionsStructVal.NumField(); i++ {
		field := permissionsStructVal.Field(i)
		if field.Kind() != reflect.Ptr ||
			field.Elem().Kind() != reflect.String ||
			field.IsNil() {
			continue
		}

		fieldValue, ok := field.Interface().(*string)
		if !ok {
			return "", errors.Errorf(
				"expected *string for field '%s', got '%T' with value '%v'",
				permissionsStructVal.Type().Field(i).Name,
				reflect.TypeOf(field.Interface()),
				field.Interface(),
			)
		}
		permissionPairs = append(permissionPairs, fmt.Sprintf("%s:%s", permissionsStructVal.Type().Field(i).Name, utility.FromStringPtr(fieldValue)))
	}
	concatenatedPermissions := strings.ToLower(strings.Join(permissionPairs, "_"))
	if concatenatedPermissions != "" {
		id += "_" + concatenatedPermissions
	}
	return id, nil
}
