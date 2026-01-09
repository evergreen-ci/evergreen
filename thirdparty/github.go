package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/cache"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/evergreen-ci/utility/ttlcache"
	"github.com/gonzojive/httpcache"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	githubAccessURL = "https://github.com/login/oauth/access_token"

	Github502Error   = "502 Server Error"
	commitObjectType = "commit"
	tagObjectType    = "tag"

	GithubInvestigation        = "Github API Limit Investigation"
	PRDiffTooLargeErrorMessage = "the diff exceeded the maximum"
	GithubStatusDefaultContext = "evergreen"
)

const (
	githubEndpointAttribute    = "evergreen.github.endpoint"
	githubOwnerAttribute       = "evergreen.github.owner"
	githubRepoAttribute        = "evergreen.github.repo"
	githubRefAttribute         = "evergreen.github.ref"
	githubPathAttribute        = "evergreen.github.path"
	githubRetriesAttribute     = "evergreen.github.retries"
	githubCachedAttribute      = "evergreen.github.cached"
	githubLocalCachedAttribute = "evergreen.github.local_cached"
	githubAuthMethodAttribute  = "evergreen.github.auth_method"
)

var UnblockedGithubStatuses = []string{
	githubPRBehind,
	githubPRClean,
	githubPRDirty,
	githubPRDraft,
	githubPRHasHooks,
	githubPRUnknown,
	githubPRUnstable,
}

var githubWritePermissions = []string{
	GithubPermissionAdmin,
	GithubPermissionWrite,
}

// AllGithubPermissions is an ascending slice of GitHub
// permissions where the first element is the lowest
// permission and the last element is the highest.
var allGitHubPermissions = []string{
	GithubPermissionRead,
	GithubPermissionWrite,
	GithubPermissionAdmin,
}

const (
	// All PR statuses except for "blocked" based on statuses listed here:
	// https://docs.github.com/en/graphql/reference/enums#mergestatestatus
	// Because the pr.MergeableState is not documented, it can change without
	// notice. That's why we want to only allow fields we know to be unblocked
	// rather than simply blocking the "blocked" status. That way if it does
	// change, it doesn't fail silently.
	githubPRBehind   = "behind"
	githubPRClean    = "clean"
	githubPRDirty    = "dirty"
	githubPRDraft    = "draft"
	githubPRHasHooks = "has_hooks"
	githubPRUnknown  = "unknown"
	githubPRUnstable = "unstable"

	githubCheckRunSuccess        = "success"
	githubCheckRunFailure        = "failure"
	githubCheckRunSkipped        = "skipped"
	githubCheckRunTimedOut       = "timed_out"
	githubCheckRunActionRequired = "action_required"
	githubCheckRunCompleted      = "completed"

	GithubPermissionRead  = "read"
	GithubPermissionWrite = "write"
	GithubPermissionAdmin = "admin"
)

var (
	githubTransport http.RoundTripper

	cacheTransportMutex sync.RWMutex
	cacheTransport      *httpcache.Transport
)

type cacheControlTransport struct {
	base http.RoundTripper
}

// RoundTrip sets the [max-age] Cache-Control directive before passing the request off
// to the base transport.
//
// [max-age]: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control#max-age_2
func (t *cacheControlTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Cache-Control", "max-age=0")
	return t.base.RoundTrip(req)
}

func init() {
	// Base transport sends a request over the wire.
	baseTransport := utility.DefaultTransport()

	// Wrap in a transport that creates a new span for each request that goes over the wire.
	otelTransport := otelhttp.NewTransport(baseTransport)

	// Wrap in a transport that caches responses to reduce the number of calls that count against our rate limit.
	cacheTransport = &httpcache.Transport{
		MarkCachedResponses: true,
		Transport:           otelTransport,
	}

	// Wrap in a transport that overrides the cache-control header so we don't use Github's
	// max-age, which would have prevented us from asking if there's been a change if we requested recently.
	githubTransport = &cacheControlTransport{base: cacheTransport}
}

func initializeTransportCache() {
	cacheTransportMutex.RLock()
	if cacheTransport.ContextCache != nil || cacheTransport.Cache != nil {
		cacheTransportMutex.RUnlock()
		return
	}
	cacheTransportMutex.RUnlock()

	cacheTransportMutex.Lock()
	defer cacheTransportMutex.Unlock()

	// Make sure the cache wasn't initialized while we were waiting for the lock.
	if cacheTransport.ContextCache != nil || cacheTransport.Cache != nil {
		return
	}

	// If the Environment is not nil that means we're running in the application and we have a connection
	// to the database. Otherwise we're running in the agent and we should use an in-memory cache.
	// We could stop casing on this if we were to stop calling out to GitHub from the agent.
	if evergreen.GetEnvironment() != nil {
		cacheTransport.ContextCache = &cache.DBCache{}
	} else {
		cacheTransport.Cache = httpcache.NewMemoryCache()
	}
}

func respFromCache(resp *http.Response) bool {
	// github.com/gregjones/httpcache adds the [X-From-Cache] header when the request is
	// fulfilled from the cache.
	//
	// [X-From-Cache]: https://pkg.go.dev/github.com/gregjones/httpcache#pkg-constants
	return resp.Header.Get(httpcache.XFromCache) != ""
}

// GithubPatch stores patch data for patches create from GitHub pull requests
type GithubPatch struct {
	PRNumber      int    `bson:"pr_number"`
	BaseOwner     string `bson:"base_owner"`
	BaseRepo      string `bson:"base_repo"`
	BaseBranch    string `bson:"base_branch"`
	HeadOwner     string `bson:"head_owner"`
	HeadRepo      string `bson:"head_repo"`
	HeadBranch    string `bson:"head_branch"`
	HeadHash      string `bson:"head_hash"`
	BaseHash      string `bson:"base_hash"`
	Author        string `bson:"author"`
	AuthorUID     int    `bson:"author_uid"`
	CommitTitle   string `bson:"commit_title"`
	CommitMessage string `bson:"commit_message"`
	MergeBase     string `bson:"merge_base"`
	// the patchId to copy the definitions for for the next patch the pr creates
	RepeatPatchIdNextPatch string `bson:"repeat_patch_id_next_patch"`
}

// GithubMergeGroup stores patch data for patches created from GitHub merge groups
type GithubMergeGroup struct {
	Org        string `bson:"org"`
	Repo       string `bson:"repo"`
	BaseBranch string `bson:"base_branch"` // BaseBranch is what GitHub merges to
	HeadBranch string `bson:"head_branch"` // HeadBranch is the merge group's gh-readonly-queue branch

	// HeadSHA is the SHA of the commit at the head of the merge group. For each
	// PR in the merge group, GitHub merges the commits from that PR together,
	// so there are as many commits as there are PRs in the merge group. This is
	// only the SHA of the first commit in the merge group.
	HeadSHA string `bson:"head_sha"`
	// HeadCommit is the title of the commit at the head of the merge group. For
	// each PR in the merge group, GitHub merges the commits from that PR
	// together, so there are as many commits as there are PRs in the merge
	// group. This is only the title of the first commit in the merge group.
	HeadCommit string `bson:"head_commit"`
}

// SendGithubStatusInput is the input to the SendPendingStatusToGithub function and contains
// all the information associated with a version necessary to send a status to GitHub.
type SendGithubStatusInput struct {
	VersionId string
	Owner     string
	Repo      string
	Ref       string
	Desc      string
	Caller    string
	Context   string
}

var (
	// BSON fields for GithubPatch
	GithubPatchPRNumberKey    = bsonutil.MustHaveTag(GithubPatch{}, "PRNumber")
	GithubPatchBaseOwnerKey   = bsonutil.MustHaveTag(GithubPatch{}, "BaseOwner")
	GithubPatchBaseRepoKey    = bsonutil.MustHaveTag(GithubPatch{}, "BaseRepo")
	RepeatPatchIdNextPatchKey = bsonutil.MustHaveTag(GithubPatch{}, "RepeatPatchIdNextPatch")
)

type retryConfig struct {
	retry    bool
	retry404 bool
	// ignoreCodes are http status codes that the retry function should ignore
	// and not retry on.
	ignoreCodes []int
}

func (c *retryConfig) shouldIgnoreCode(statusCode int) bool {
	for _, ignoreCode := range c.ignoreCodes {
		if statusCode == ignoreCode {
			return true
		}
	}
	return false
}

func githubShouldRetry(caller string, config retryConfig) utility.HTTPRetryFunction {
	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		trace.SpanFromContext(req.Context()).SetAttributes(attribute.Int(githubRetriesAttribute, index))

		if !(config.retry || config.retry404) {
			return false
		}

		if index >= githubapp.GitHubMaxRetries {
			return false
		}

		url := req.URL.String()

		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				grip.Error(message.WrapError(err, message.Fields{
					"message":   "EOF error from github",
					"method":    req.Method,
					"url":       url,
					"retry_num": index,
				}))
				return true
			}
			temporary := utility.IsTemporaryError(err)
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "failed trying to call github",
				"method":    req.Method,
				"url":       url,
				"temporary": temporary,
			}))
			grip.InfoWhen(temporary, message.Fields{
				"ticket":    GithubInvestigation,
				"message":   "error is temporary",
				"caller":    caller,
				"retry_num": index,
			})
			return temporary
		}

		if resp == nil {
			grip.Info(message.Fields{
				"ticket":    GithubInvestigation,
				"message":   "resp is nil in githubShouldRetry",
				"caller":    caller,
				"retry_num": index,
			})
			return true
		}

		if config.shouldIgnoreCode(resp.StatusCode) {
			return false
		}

		if resp.StatusCode >= http.StatusBadRequest {
			grip.Error(message.Fields{
				"message": "bad response code from github",
				"method":  req.Method,
				"url":     url,
				"outcome": resp.StatusCode,
			})
		}

		limit := parseGithubRateLimit(resp.Header)
		if limit.Remaining == 0 {
			return false
		}

		if resp.StatusCode == http.StatusBadGateway {
			grip.Info(message.Fields{
				"ticket":    GithubInvestigation,
				"message":   fmt.Sprintf("hit %d in githubShouldRetry", http.StatusBadGateway),
				"caller":    caller,
				"retry_num": index,
			})
			return true
		}

		if config.retry404 && resp.StatusCode == http.StatusNotFound {
			grip.Info(message.Fields{
				"ticket":    GithubInvestigation,
				"message":   fmt.Sprintf("hit %d in githubShouldRetry", http.StatusNotFound),
				"caller":    caller,
				"retry_num": index,
			})
			return true
		}

		return false
	}
}

// getGithubClient returns a client that provides the given token, retries requests,
// caches responses, and creates a span for each request.
// Couple this with a deferred call with Close() to clean up the client.
func getGithubClient(token, caller string, config retryConfig) *githubapp.GitHubClient {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClient",
		"caller":  caller,
	})

	initializeTransportCache()

	httpClient := utility.GetHTTPClient()
	httpClient.Transport = githubTransport

	client := utility.SetupOauth2CustomHTTPRetryableClient(
		token,
		githubShouldRetry(caller, config),
		utility.RetryHTTPDelay(utility.RetryOptions{
			MaxAttempts: githubapp.GitHubMaxRetries,
			MinDelay:    githubapp.GitHubRetryMinDelay,
		}),
		httpClient,
	)

	githubClient := githubapp.GitHubClient{Client: github.NewClient(client)}
	return &githubClient
}

// defaultGitHubAPIRequestLifetime is the default amount of time that an
// installation token is valid for a single GitHub API request.
const defaultGitHubAPIRequestLifetime = 15 * time.Minute

// getInstallationToken creates an installation token using Github app auth.
// If creating a token fails it will return the legacyToken.
func getInstallationToken(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting config")
	}

	return githubapp.CreateGitHubAppAuth(settings).CreateCachedInstallationToken(ctx, owner, repo, defaultGitHubAPIRequestLifetime, opts)
}

// RevokeInstallationToken revokes an installation token. Take care to make sure
// that the token being revoked is not cached - Evergreen sometimes caches
// tokens for reuse (e.g. for making GitHub API calls), and revoking a cached
// token could cause running GitHub operations to fail.
func RevokeInstallationToken(ctx context.Context, token string) error {
	caller := "RevokeInstallationToken"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	// Ignore unauthorized responses since the token may have already been revoked.
	githubClient := getGithubClient(token, caller, retryConfig{retry: true, ignoreCodes: []int{http.StatusUnauthorized}})
	defer githubClient.Close()
	resp, err := githubClient.Apps.RevokeInstallationToken(ctx)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Int("status", resp.StatusCode))
	}
	if err != nil {
		span.SetAttributes(attribute.String("err", err.Error()))
	}

	return err
}

func getInstallationTokenWithDefaultOwnerRepo(ctx context.Context, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting evergreen settings")
	}

	if settings.AuthConfig.Github == nil {
		settings = evergreen.GetEnvironment().Settings()
		grip.Info("no Github settings in auth config, using cached settings")
	}

	return githubapp.CreateCachedInstallationTokenWithDefaultOwnerRepo(ctx, settings, defaultGitHubAPIRequestLifetime, opts)
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, owner, repo, ref string, until time.Time, commitPage int) ([]*github.RepositoryCommit, int, error) {
	caller := "GetGithubCommits"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	options := github.CommitsListOptions{
		SHA: ref,
		ListOptions: github.ListOptions{
			Page: commitPage,
		},
	}
	if !utility.IsZeroTime(until) {
		options.Until = until
	}

	commits, resp, err := githubClient.Repositories.ListCommits(ctx, owner, repo, &options)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, 0, parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from query for commits in '%s/%s' ref %s : %v", owner, repo, ref, err)
		grip.Error(errMsg)
		return nil, 0, APIResponseError{errMsg}
	}

	return commits, resp.NextPage, nil
}

func parseGithubErrorResponse(resp *github.Response) error {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ResponseReadError{err.Error()}
	}
	requestError := APIRequestError{StatusCode: resp.StatusCode}
	if err = json.Unmarshal(respBody, &requestError); err != nil {
		return APIRequestError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}
	return requestError
}

// GetGithubFile returns a struct that contains the contents of files within
// a repository as Base64 encoded content. Ref should be the commit hash or branch (defaults to master).
func GetGithubFile(ctx context.Context, owner, repo, path, ref string, ghAppAuth *githubapp.GithubAppAuth) (*github.RepositoryContent, error) {
	if path == "" {
		return nil, errors.New("remote repository path cannot be empty")
	}
	caller := "GetGithubFile"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
		attribute.String(githubPathAttribute, path),
	))
	defer span.End()

	var outputFile *github.RepositoryContent
	if err := runGitHubOp(ctx, owner, repo, caller, ghAppAuth, func(ctx context.Context, githubClient *githubapp.GitHubClient) error {
		var opt *github.RepositoryContentGetOptions
		if len(ref) != 0 {
			opt = &github.RepositoryContentGetOptions{
				Ref: ref,
			}
		}

		file, _, resp, err := githubClient.Repositories.GetContents(ctx, owner, repo, path, opt)
		if resp != nil {
			defer resp.Body.Close()
			span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
			if resp.StatusCode == http.StatusNotFound {
				return FileNotFoundError{filepath: path}
			}
			if err != nil {
				return parseGithubErrorResponse(resp)
			}
		} else {
			errMsg := fmt.Sprintf("nil response from github for '%s/%s' for '%s': %v", owner, repo, path, err)
			grip.Error(errMsg)
			return APIResponseError{errMsg}
		}

		if file == nil || file.Content == nil {
			return APIRequestError{Message: "file is nil"}
		}

		outputFile = file

		return nil
	}); err != nil {
		return nil, err
	}

	return outputFile, nil
}

// runGitHubOp attempts to run the given GitHub operation. It first attempts
// with GitHub app to authenticate (if provided). If that fails (e.g. due to
// insufficient project token permissions) or no app is available, it falls back
// to using the internal app for installation tokens.
func runGitHubOp(ctx context.Context, owner, repo, caller string, ghAppAuth *githubapp.GithubAppAuth, op func(ctx context.Context, ghClient *githubapp.GitHubClient) error) error {
	if ghAppAuth != nil {
		err := runGitHubOpWithExternalGitHubApp(ctx, owner, repo, caller, ghAppAuth, op)
		if err == nil {
			grip.Info(message.Fields{
				"message": "GitHub op suceeded with external GitHub app",
				"caller":  caller,
				"owner":   owner,
				"repo":    repo,
			})
			return nil
		}

		grip.Warning(message.WrapError(err, message.Fields{
			"message": "GitHub operation with external GitHub app failed, falling back to attempt with internal app",
			"caller":  caller,
			"owner":   owner,
			"repo":    repo,
		}))
	}

	// Fall back to using the internal app if the project does not have a token
	// available or if the operation with the project token failed. This is
	// needed because the project's GitHub app may have insufficient permissions
	// to perform the operation, whereas the internal app has broad permissions.
	ctx, span := tracer.Start(ctx, "github-op-with-internal-app", trace.WithAttributes(
		attribute.String(githubAuthMethodAttribute, "internal_app"),
	))
	defer span.End()

	internalToken, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		span.RecordError(err)
		return errors.Wrap(err, "getting installation token")
	}
	internalGHClient := getGithubClient(internalToken, caller, retryConfig{retry: true})
	defer internalGHClient.Close()

	err = op(ctx, internalGHClient)
	return err
}

func runGitHubOpWithExternalGitHubApp(ctx context.Context, owner, repo, caller string, ghAppAuth *githubapp.GithubAppAuth, op func(ctx context.Context, ghClient *githubapp.GitHubClient) error) error {
	token, err := ghAppAuth.CreateCachedInstallationToken(ctx, owner, repo, defaultGitHubAPIRequestLifetime, nil)
	if err != nil {

	}
	ctx, span := tracer.Start(ctx, "github-op-with-external-app", trace.WithAttributes(
		attribute.String(githubAuthMethodAttribute, "external_app"),
	))
	defer span.End()

	// This intentionally does not retry because if the GitHub app doesn't have
	// the necessary permissions or has other issues like rate limits, then it's
	// better to fall back to the internal app immediately.
	ghClient := getGithubClient(token, caller, retryConfig{})
	defer ghClient.Close()

	return op(ctx, ghClient)
}

// SendPendingStatusToGithub sends a pending status to a Github PR patch
// associated with a given version.
func SendPendingStatusToGithub(ctx context.Context, input SendGithubStatusInput, urlBase string) error {
	ctx, span := tracer.Start(ctx, "send-pending-status-to-github")
	defer span.End()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     input.Caller,
			"message": "GitHub status updates are disabled, not updating status",
		})
		return nil
	}
	env := evergreen.GetEnvironment()
	if urlBase == "" {
		uiConfig := evergreen.UIConfig{}
		if err = uiConfig.Get(ctx); err != nil {
			return errors.Wrap(err, "retrieving UI config")
		}
		urlBase := uiConfig.Url
		if urlBase == "" {
			return errors.New("url base doesn't exist")
		}
	}
	status := &message.GithubStatus{
		Owner:       input.Owner,
		Repo:        input.Repo,
		Ref:         input.Ref,
		URL:         fmt.Sprintf("%s/version/%s", urlBase, input.VersionId),
		Context:     input.Context,
		State:       message.GithubStatePending,
		Description: input.Desc,
	}

	sender, err := env.GetGitHubSender(input.Owner, input.Repo, githubapp.CreateGitHubAppAuth(env.Settings()).CreateGitHubSenderInstallationToken)
	if err != nil {
		return errors.Wrap(err, "getting github status sender")
	}

	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		return errors.Errorf("status message is invalid: %+v", status)
	}

	if err = c.SetPriority(level.Notice); err != nil {
		return errors.Wrap(err, "setting priority")
	}

	sender.Send(c)
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called github status send",
		"caller":  "github check subscriptions",
		"version": input.VersionId,
	})
	return nil
}

// GetGithubMergeBaseRevision compares baseRevision and currentCommitHash in a
// GitHub repo and returns the merge base commit's SHA.
func GetGithubMergeBaseRevision(ctx context.Context, owner, repo, baseRevision, currentCommitHash string) (string, error) {
	caller := "GetGithubMergeBaseRevision"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()
	compare, err := getCommitComparison(ctx, owner, repo, baseRevision, currentCommitHash, caller)
	if err != nil {
		return "", errors.Wrapf(err, "retreiving comparison between commit hashses '%s' and '%s'", baseRevision, currentCommitHash)
	}
	return *compare.MergeBaseCommit.SHA, nil
}

// IsMergeBaseAllowed will return true if the input merge base is newer than the oldest allowed merge base
// configured in the project settings.
func IsMergeBaseAllowed(ctx context.Context, owner, repo, oldestAllowedMergeBase, mergeBase string) (bool, error) {
	caller := "IsMergeBaseAllowed"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()
	compare, err := getCommitComparison(ctx, owner, repo, mergeBase, oldestAllowedMergeBase, caller)
	if err != nil {
		return false, errors.Wrapf(err, "retreiving comparison between commit hashses '%s' and '%s'", oldestAllowedMergeBase, mergeBase)
	}
	status := compare.GetStatus()

	// Per the API docs: https://docs.github.com/en/rest/commits/commits?apiVersion=2022-11-28#compare-two-commits
	// Valid statuses are within the enum of "diverged", "ahead", "behind", "identical"
	// The input merge base is allowed if the oldest allowed merge base is at or behind it
	return status == "behind" || status == "identical", nil
}

func getCommitComparison(ctx context.Context, owner, repo, baseRevision, currentCommitHash, caller string) (*github.CommitsComparison, error) {
	span := trace.SpanFromContext(ctx)

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	compare, resp, err := githubClient.Repositories.CompareCommits(ctx,
		owner, repo, baseRevision, currentCommitHash, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		apiErr := errors.Errorf("nil response from merge base commit response for '%s/%s'@%s..%s: %v", owner, repo, baseRevision, currentCommitHash, err)
		grip.Error(message.WrapError(apiErr, message.Fields{
			"message":             "failed to compare commits to determine order of commits",
			"op":                  "getCommitComparison",
			"github_error":        fmt.Sprint(err),
			"repo":                repo,
			"base_revision":       baseRevision,
			"current_commit_hash": currentCommitHash,
		}))
		return nil, APIResponseError{apiErr.Error()}
	}
	if compare == nil || compare.MergeBaseCommit == nil || compare.MergeBaseCommit.SHA == nil {
		return nil, APIRequestError{Message: "missing data from GitHub compare response"}
	}
	return compare, nil
}

// ghCommitCache is a weak cache for GitHub commits. We can use a cache because
// the commit data doesn't change.
var ghCommitCache = ttlcache.WithOtel(ttlcache.NewWeakInMemory[github.RepositoryCommit](), "github-get-commit-event")

func GetCommitEvent(ctx context.Context, owner, repo, githash string) (*github.RepositoryCommit, error) {
	caller := "GetCommitEvent"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, githash),
	))
	defer span.End()

	ghCommitKey := fmt.Sprintf("%s/%s/%s", owner, repo, githash)
	commit, found := ghCommitCache.Get(ctx, ghCommitKey, 0)
	span.SetAttributes(attribute.Bool(githubLocalCachedAttribute, found))
	if found && commit != nil {
		return commit, nil
	}

	var err error
	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    owner + "/" + repo,
	})

	commit, resp, err := githubClient.Repositories.GetCommit(ctx, owner, repo, githash, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		err = errors.Wrapf(err, "nil response from repo %s/%s for %s", owner, repo, githash)
		grip.Error(message.WrapError(errors.Cause(err), message.Fields{
			"commit":  githash,
			"repo":    owner + "/" + repo,
			"message": "problem querying repo",
		}))
		return nil, APIResponseError{err.Error()}
	}

	msg := message.Fields{
		"operation": "github api query",
		"size":      resp.ContentLength,
		"status":    resp.Status,
		"query":     githash,
		"repo":      owner + "/" + repo,
	}
	if commit != nil && commit.SHA != nil {
		msg["commit"] = *commit.SHA
	}
	grip.Debug(msg)

	if commit == nil {
		return nil, errors.New("commit not found in github")
	}

	// We use 24 hours as the expiration time for the item in the cache
	// because the cache only holds weak pointers to the items in it.
	// This means that the items in the cache can be garbage collected
	// if there are no strong references to them.
	ghCommitCache.Put(ctx, ghCommitKey, commit, time.Now().Add(time.Hour*24))
	return commit, nil
}

// GetBranchEvent gets the head of the a given branch via an API call to GitHub
func GetBranchEvent(ctx context.Context, owner, repo, branch string) (*github.Branch, error) {
	caller := "GetBranchEvent"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", owner, repo, branch)

	branchEvent, resp, err := githubClient.Repositories.GetBranch(ctx, owner, repo, branch, 0)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from github for '%s/%s': branch: '%s': %v", owner, repo, branch, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}

	return branchEvent, nil
}

// githubRequest performs the specified http request. If the oauth token field is empty it will not use oauth
func githubRequest(ctx context.Context, method string, url string, oauthToken string, data any) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	// if there is data, add it to the body of the request
	if data != nil {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(jsonBytes))
	}

	// check if there is an oauth token, if there is make sure it is a valid oauthtoken
	if len(oauthToken) > 0 {
		if !strings.HasPrefix(oauthToken, "token ") {
			return nil, errors.New("Invalid oauth token given")
		}
		req.Header.Add("Authorization", oauthToken)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	return client.Do(req)
}

// tryGithubPost posts the data to the Github api endpoint with the url given
func tryGithubPost(ctx context.Context, url string, oauthToken string, data any) (resp *http.Response, err error) {
	err = utility.Retry(ctx, func() (bool, error) {
		grip.Info(message.Fields{
			"message": "Attempting GitHub API POST",
			"ticket":  GithubInvestigation,
			"url":     url,
		})
		resp, err = githubRequest(ctx, http.MethodPost, url, oauthToken, data)
		if err != nil {
			grip.Errorf("failed trying to call github POST on %s: %+v", url, err)
			return true, err
		}
		if resp.StatusCode == http.StatusUnauthorized {
			err = errors.Errorf("Calling github POST on %v failed: got 'unauthorized' response", url)
			defer resp.Body.Close()
			grip.Error(err)
			return false, err
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			err = errors.Errorf("Calling github POST on %v got a bad response code: %v", url, resp.StatusCode)
		}

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: githubapp.GitHubMaxRetries,
		MinDelay:    githubapp.GitHubRetryMinDelay,
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return
}

func parseGithubRateLimit(h http.Header) github.Rate {
	var rate github.Rate
	if limit := h.Get("X-Ratelimit-Limit"); limit != "" {
		rate.Limit, _ = strconv.Atoi(limit)
	}
	if remaining := h.Get("X-Ratelimit-Remaining"); remaining != "" {
		rate.Remaining, _ = strconv.Atoi(remaining)
	}
	if reset := h.Get("X-RateLimit-Reset"); reset != "" {
		if v, _ := strconv.ParseInt(reset, 10, 64); v != 0 {
			rate.Reset = github.Timestamp{Time: time.Unix(v, 0)}
		}
	}

	return rate
}

// GithubAuthenticate does a POST to github with the code that it received, the ClientId, ClientSecret
// And returns the response which contains the accessToken associated with the user.
func GithubAuthenticate(ctx context.Context, code, clientId, clientSecret string) (githubResponse *GithubAuthResponse, err error) {
	// Functionality not supported by go-github
	authParameters := GithubAuthParameters{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Code:         code,
	}
	resp, err := tryGithubPost(ctx, githubAccessURL, "", authParameters)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "could not authenticate for token")
	}
	if resp == nil {
		return nil, errors.New("invalid github response")
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}

	if err = json.Unmarshal(respBody, &githubResponse); err != nil {
		return nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return
}

// GetTaggedCommitFromGithub gets the commit SHA for the given tag name.
func GetTaggedCommitFromGithub(ctx context.Context, owner, repo, tag string) (string, error) {
	caller := "GetTaggedCommitFromGithub"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, tag),
	))
	defer span.End()

	var err error
	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()
	ref, resp, err := githubClient.Git.GetRef(ctx, owner, repo, tag)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return "", parseGithubErrorResponse(resp)
		}
	}
	if err != nil {
		return "", errors.Wrapf(err, "error getting tag for ref '%s'", ref)
	}

	var sha string
	tagSha := ref.GetObject().GetSHA()
	switch ref.GetObject().GetType() {
	case commitObjectType:
		// lightweight tags are pointers to the commit itself
		sha = tagSha
	case tagObjectType:
		annotatedTag, err := getObjectTag(ctx, owner, repo, tagSha)
		if err != nil {
			return "", errors.Wrapf(err, "error getting tag '%s' with SHA '%s'", tag, tagSha)
		}
		sha = annotatedTag.GetObject().GetSHA()
	default:
		return "", errors.Errorf("unrecognized object type '%s'", ref.GetObject().GetType())
	}

	if tagSha == "" {
		return "", errors.New("empty SHA from GitHub")
	}

	return sha, nil
}

func getObjectTag(ctx context.Context, owner, repo, sha string) (*github.Tag, error) {
	caller := "getObjectTag"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	))
	defer span.End()

	var err error
	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	tag, resp, err := githubClient.Git.GetTag(ctx, owner, repo, sha)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error getting tag with SHA '%s'", sha)
	}

	return tag, nil
}
func IsUserInGithubTeam(ctx context.Context, teams []string, org, user, owner, repo string) bool {
	inTeam, _ := userInTeam(ctx, teams, org, user, owner, repo)
	return inTeam
}

func userInTeam(ctx context.Context, teams []string, org, user, owner, repo string) (bool, error) {
	caller := "IsUserInGithubTeam"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	var err error
	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return false, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "number of teams in IsUserInGithubTeam",
		"teams":   len(teams),
	})
	for _, team := range teams {
		membership, resp, err := githubClient.Teams.GetTeamMembershipBySlug(ctx, org, team, user)
		if resp != nil {
			defer resp.Body.Close()
			span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
			if err != nil {
				return false, parseGithubErrorResponse(resp)
			}
		}
		if err != nil {
			return false, errors.Wrapf(err, "error getting membership for user '%s' in team '%s'", user, team)
		}
		if membership != nil && membership.GetState() == "active" {
			return true, nil
		}
	}
	return false, nil
}

// GetGithubTokenUser fetches a github user associated with an oauth token, and
// if requiredOrg is specified, checks that it belongs to that org.
// Returns user object, if it was a member of the specified org (or false if not specified),
// and error
func GetGithubTokenUser(ctx context.Context, token string, requiredOrg string) (*GithubLoginUser, bool, error) {
	caller := "GetGithubTokenUser"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	githubClient := getGithubClient(fmt.Sprintf("token %s", token), caller, retryConfig{retry: true})
	defer githubClient.Close()

	user, resp, err := githubClient.Users.Get(ctx, "")
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			var respBody []byte
			respBody, err = io.ReadAll(resp.Body)
			if err != nil {
				return nil, false, ResponseReadError{err.Error()}
			}
			return nil, false, APIResponseError{string(respBody)}
		}
	} else {
		return nil, false, errors.WithStack(err)

	}

	var isMember bool
	if len(requiredOrg) > 0 {
		isMember, _, err = githubClient.Organizations.IsMember(ctx, requiredOrg, *user.Login)
		if err != nil {
			return nil, false, errors.Wrapf(err, "Could check if user was org member")
		}
	}

	if user.Login == nil || user.ID == nil || user.Company == nil ||
		user.Email == nil || user.OrganizationsURL == nil {
		return nil, false, errors.New("GitHub user is missing required data")
	}

	return &GithubLoginUser{
		Login:            *user.Login,
		Id:               int(*user.ID),
		Company:          *user.Company,
		EmailAddress:     *user.Email,
		OrganizationsURL: *user.OrganizationsURL,
	}, isMember, err
}

// CheckGithubAPILimit queries Github for all the API rate limits
func CheckGithubAPILimit(ctx context.Context) (*github.RateLimits, error) {
	caller := "CheckGithubAPILimit"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	token, err := getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	limits, resp, err := githubClient.RateLimit.Get(ctx)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		grip.Errorf("github GET rate limit failed: %+v", err)
		return nil, err
	}
	if limits.Core == nil {
		return nil, errors.New("nil github limits")
	}

	return limits, nil
}

// CheckGithubResource queries Github for the number of API requests remaining
func CheckGithubResource(ctx context.Context) (int64, error) {
	limits, err := CheckGithubAPILimit(ctx)
	if err != nil {
		return int64(0), errors.Wrap(err, "getting github rate limit")
	}
	if limits.Core.Remaining < 0 {
		return int64(0), nil
	}

	return int64(limits.Core.Remaining), nil
}

// GetGithubUser fetches the github user with the given login name
func GetGithubUser(ctx context.Context, loginName string) (*github.User, error) {
	caller := "GetGithubUser"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	token, err := getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	user, resp, err := githubClient.Users.Get(ctx, loginName)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, err
	}

	if user == nil || user.ID == nil || user.Login == nil {
		return nil, errors.New("empty data received from github")
	}

	return user, nil
}

// GithubUserInOrganization returns true if the given github user is in the
// given organization. The user with the attached token must have
// visibility into organization membership, including private members
func GithubUserInOrganization(ctx context.Context, requiredOrganization, username string) (bool, error) {
	caller := "GithubUserInOrganization"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	token, err := getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
	if err != nil {
		return false, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	isMember, resp, err := githubClient.Organizations.IsMember(context.Background(), requiredOrganization, username)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	return isMember, err
}

// AppAuthorizedForOrg returns true if the given app name exists in the org's installation list,
// and has permission to write to pull requests. Returns an error if the app name exists but doesn't have permission.
func AppAuthorizedForOrg(ctx context.Context, requiredOrganization, name string) (bool, error) {
	caller := "AppAuthorizedForOrg"
	const botSuffix = "[bot]"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	token, err := getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
	if err != nil {
		return false, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	// GitHub often appends [bot] to GitHub App usage, but this doesn't match the App slug, so we should check without this.
	nameWithoutBotSuffix := strings.TrimSuffix(name, botSuffix)
	opts := &github.ListOptions{PerPage: 100}
	for {
		installations, resp, err := githubClient.Organizations.ListInstallations(ctx, requiredOrganization, opts)
		if err != nil {
			return false, err
		}
		if resp != nil {
			defer resp.Body.Close()
		}

		for _, installation := range installations.Installations {
			appSlug := installation.GetAppSlug()
			if appSlug == name || appSlug == nameWithoutBotSuffix {
				prPermission := installation.GetPermissions().GetPullRequests()
				if utility.StringSliceContains(githubWritePermissions, prPermission) {
					return true, nil
				}
				return false, errors.Errorf("app '%s' is installed but has pull request permission '%s'", name, prPermission)
			}
		}

		if resp != nil && resp.NextPage > 0 {
			opts.Page = resp.NextPage
		} else {
			break
		}
	}

	return false, nil
}

// GitHubUserHasWritePermission returns true if the given user has write permission for the repo.
// Returns an error if the user isn't found or the token isn't authed for this repo.
func GitHubUserHasWritePermission(ctx context.Context, owner, repo, username string) (bool, error) {
	caller := "userHasWritePermission"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return false, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})
	defer githubClient.Close()

	permissionLevel, resp, err := githubClient.Repositories.GetPermissionLevel(ctx, owner, repo, username)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return false, errors.Wrap(err, "can't get permissions from GitHub")
	}

	if permissionLevel == nil || permissionLevel.Permission == nil {
		return false, errors.Errorf("GitHub returned an invalid response to request for user permissions for '%s'", username)
	}

	return utility.StringSliceContains(githubWritePermissions, permissionLevel.GetPermission()), nil
}

// ValidateGitHubPermission checks if the given permission is a valid GitHub permission.
func ValidateGitHubPermission(permission string) error {
	if !utility.StringSliceContains(allGitHubPermissions, permission) && permission != "" {
		return errors.Errorf("invalid GitHub permission '%s'", permission)
	}
	return nil
}

// MostRestrictiveGitHubPermission returns the permission from the two given permissions that is the most restrictive.
// This function assumes that the given permissions are valid GitHub permissions or empty strings (which is no
// permissions).
func MostRestrictiveGitHubPermission(perm1, perm2 string) string {
	// Most restrictive permission is no permissions.
	if perm1 == "" || perm2 == "" {
		return ""
	}
	// AllGitHubPermissions is ordered from most to least restrictive.
	for _, perm := range allGitHubPermissions {
		if perm1 == perm || perm2 == perm {
			return perm
		}
	}
	return ""
}

// GetPullRequestMergeBase returns the merge base hash for the given PR.
// This function will retry up to 5 times, regardless of error response (unless
// error is the result of hitting an api limit)
func GetPullRequestMergeBase(ctx context.Context, owner, repo, baseLabel, headLabel string, prNum int) (string, error) {
	mergeBase, err := GetGithubMergeBaseRevision(ctx, owner, repo, baseLabel, headLabel)
	if err == nil {
		return mergeBase, nil
	}
	grip.Error(message.WrapError(err, message.Fields{
		"message": "GetGithubMergeBaseRevision failed, falling back to secondary method of determining merge base",
		"owner":   owner,
		"repo":    repo,
		"head":    headLabel,
		"pr_num":  prNum,
		"base":    baseLabel,
	}))
	// If GetGithubMergeBaseRevision fails, fallback to the secondary way of determining a PR
	// merge base via API. A known case where we expect GetGithubMergeBaseRevision to fail is when
	// trying to find the merge base of a PR based on a private fork that our 10gen GitHub app is not
	// installed on.
	caller := "GetPullRequestMergeBase"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return "", errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})
	defer githubClient.Close()

	commits, resp, err := githubClient.PullRequests.ListCommits(ctx, owner, repo, prNum, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return "", err
	}

	if len(commits) == 0 {
		return "", errors.New("No commits received from github")
	}
	if commits[0].GetSHA() == "" {
		return "", errors.New("hash is missing from pull request commit list")
	}

	commit, err := getCommit(ctx, owner, repo, *commits[0].SHA)
	if err != nil {
		return "", errors.Wrapf(err, "getting commit on %s/%s with SHA '%s'", owner, repo, *commits[0].SHA)
	}
	if len(commit.Parents) == 0 {
		return "", errors.New("can't find pull request branch point")
	}
	if commit.Parents[0].GetSHA() == "" {
		return "", errors.New("parent hash is missing")
	}

	return commit.Parents[0].GetSHA(), nil
}

func getCommit(ctx context.Context, owner, repo, sha string) (*github.RepositoryCommit, error) {
	caller := "getCommit"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})
	defer githubClient.Close()

	commit, resp, err := githubClient.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, err
	}
	if commit == nil {
		return nil, errors.New("couldn't find commit")
	}

	return commit, nil
}

func GetGithubPullRequest(ctx context.Context, baseOwner, baseRepo string, prNumber int) (*github.PullRequest, error) {
	caller := "GetGithubPullRequest"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, baseOwner),
		attribute.String(githubRepoAttribute, baseRepo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, baseOwner, baseRepo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})
	defer githubClient.Close()

	pr, resp, err := githubClient.PullRequests.Get(ctx, baseOwner, baseRepo, prNumber)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// GetGithubPullRequestDiff downloads a raw diff from a Github Pull Request.
// This can fail if the PR is too large (e.g. greater than 300 files). See
// GetGitHubPullRequestFiles.
func GetGithubPullRequestDiff(ctx context.Context, gh GithubPatch) (string, []Summary, error) {
	caller := "GetGithubPullRequestDiff"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, gh.BaseOwner),
		attribute.String(githubRepoAttribute, gh.BaseRepo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, gh.BaseOwner, gh.BaseRepo, nil)
	if err != nil {
		return "", nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})
	defer githubClient.Close()

	diff, resp, err := githubClient.PullRequests.GetRaw(ctx, gh.BaseOwner, gh.BaseRepo, gh.PRNumber, github.RawOptions{Type: github.Diff})
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return "", nil, err
	}
	summaries, err := GetPatchSummaries(diff)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get patch summary")
	}

	return diff, summaries, nil
}

// MaxGitHubPRFilesListLength is the maximum number of files that can be
// returned by the GitHub API for a pull request. This is a hard limit imposed
// by the GitHub API. If the PR changes more than 3000 files, Evergreen will
// only see at most 3000 of them.
const MaxGitHubPRFilesListLength = 3000

// GetGitHubPullRequestFiles gets the summary list of the changed files for the
// given pull request. Due to GitHub limitations, this can get at most 3000
// changed files. If it hits the 3000 file retrieval limit, it'll return the
// first 3000 files and no error; callers are expected to handle that limit
// appropriately.
func GetGitHubPullRequestFiles(ctx context.Context, gh GithubPatch) ([]Summary, error) {
	owner := gh.BaseOwner
	repo := gh.BaseRepo
	caller := "GetGitHubPullRequestFiles"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	// Return the most files possible per request (100) to reduce API calls.
	const maxFilesPerPage = 100
	const maxNumRequestsUntilNoFiles = MaxGitHubPRFilesListLength / maxFilesPerPage

	page := 1
	var allSummaries []Summary
	for range maxNumRequestsUntilNoFiles {
		opts := &github.ListOptions{
			PerPage: maxFilesPerPage,
			Page:    page,
		}

		summaries, err := getPullRequestFileSummaries(ctx, githubClient, gh, opts)
		if err != nil {
			return nil, err
		}
		if len(summaries) == 0 {
			// No more files or this is the last page of files.
			break
		}

		allSummaries = append(allSummaries, summaries...)
		page++
	}

	return allSummaries, nil
}

func getPullRequestFileSummaries(ctx context.Context, ghClient *githubapp.GitHubClient, gh GithubPatch, opts *github.ListOptions) ([]Summary, error) {
	files, resp, err := ghClient.PullRequests.ListFiles(ctx, gh.BaseOwner, gh.BaseRepo, gh.PRNumber, opts)
	if resp != nil {
		defer resp.Body.Close()
		trace.SpanFromContext(ctx).SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrapf(err, "getting page %d of pull request files", opts.Page)
	}

	return getPatchSummariesFromCommitFiles(files), nil
}

func getPatchSummariesFromCommitFiles(files []*github.CommitFile) []Summary {
	summaries := make([]Summary, 0, len(files))
	for _, file := range files {
		summary := Summary{
			Name:      file.GetFilename(),
			Additions: file.GetAdditions(),
			Deletions: file.GetDeletions(),
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func ValidatePR(pr *github.PullRequest) error {
	if pr == nil {
		return errors.New("No PR provided")
	}

	catcher := grip.NewSimpleCatcher()
	if missingUserLogin(pr) {
		catcher.Add(errors.New("no valid user"))
	}
	if missingBaseSHA(pr) {
		catcher.Add(errors.New("no valid base SHA"))
	}
	if missingBaseRef(pr) {
		catcher.Add(errors.New("no valid base ref"))
	}
	if missingBaseRepoName(pr) {
		catcher.Add(errors.New("no valid base repo name"))
	}
	if missingBaseRepoFullName(pr) {
		catcher.Add(errors.New("no valid base repo name"))
	}
	if missingBaseRepoOwnerLogin(pr) {
		catcher.Add(errors.New("no valid base repo owner login"))
	}
	if missingHeadSHA(pr) {
		catcher.Add(errors.New("no valid head SHA"))
	}
	if pr.GetNumber() == 0 {
		catcher.Add(errors.New("no valid pr number"))
	}
	if pr.GetTitle() == "" {
		catcher.Add(errors.New("no valid title"))
	}
	if pr.GetHTMLURL() == "" {
		catcher.Add(errors.New("no valid HTML URL"))
	}
	if pr.Merged == nil {
		catcher.Add(errors.New("no valid merged status"))
	}

	return catcher.Resolve()
}

func missingUserLogin(pr *github.PullRequest) bool {
	return pr.User == nil || pr.User.GetLogin() == ""
}

func missingBaseSHA(pr *github.PullRequest) bool {
	return pr.Base == nil || pr.Base.GetSHA() == ""
}

func missingBaseRef(pr *github.PullRequest) bool {
	return pr.Base == nil || pr.Base.GetRef() == ""
}

func missingBaseRepoName(pr *github.PullRequest) bool {
	return pr.Base == nil || pr.Base.Repo == nil || pr.Base.Repo.GetName() == "" || pr.Base.Repo.GetFullName() == ""
}

func missingBaseRepoFullName(pr *github.PullRequest) bool {
	return pr.Base == nil || pr.Base.Repo == nil || pr.Base.Repo.GetFullName() == ""
}

func missingBaseRepoOwnerLogin(pr *github.PullRequest) bool {
	return pr.Base == nil || pr.Base.Repo == nil || pr.Base.Repo.Owner == nil || pr.Base.Repo.Owner.GetLogin() == ""
}

func missingHeadSHA(pr *github.PullRequest) bool {
	return pr.Head == nil || pr.Head.GetSHA() == ""
}

// PostCommentToPullRequest posts the given comment to the associated PR.
func PostCommentToPullRequest(ctx context.Context, owner, repo string, prNum int, comment string) error {
	caller := "PostCommentToPullRequest"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{})
	defer githubClient.Close()

	githubComment := &github.IssueComment{
		Body: &comment,
	}
	respComment, resp, err := githubClient.Issues.CreateComment(ctx, owner, repo, prNum, githubComment)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return errors.Wrap(err, "can't access GitHub merge API")
	}
	if resp != nil && resp.StatusCode != http.StatusCreated || respComment == nil || respComment.ID == nil {
		return errors.New("unexpected data from GitHub")
	}
	return nil
}

// GetEvergreenBranchProtectionRules gets all Evergreen branch protection checks as a list of strings.
func GetEvergreenBranchProtectionRules(ctx context.Context, owner, repo, branch string) ([]string, error) {
	branchProtectionRules, err := GetBranchProtectionRules(ctx, owner, repo, branch)
	if err != nil {
		return nil, err
	}
	return getRulesWithEvergreenPrefix(branchProtectionRules), nil
}

func getRulesWithEvergreenPrefix(rules []string) []string {
	rulesWithEvergreenPrefix := []string{}
	for _, rule := range rules {
		if strings.HasPrefix(rule, GithubStatusDefaultContext) {
			rulesWithEvergreenPrefix = append(rulesWithEvergreenPrefix, rule)
		}
	}
	return rulesWithEvergreenPrefix
}

// GetBranchProtectionRules gets all branch protection checks as a list of strings.
func GetBranchProtectionRules(ctx context.Context, owner, repo, branch string) ([]string, error) {
	caller := "GetBranchProtection"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{})
	defer githubClient.Close()

	protection, resp, err := githubClient.Repositories.GetBranchProtection(ctx, owner, repo, branch)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "can't get branch protection rules")
	}
	checks := []string{}
	if requiredStatusChecks := protection.GetRequiredStatusChecks(); requiredStatusChecks != nil {
		if requiredStatusChecks.Checks != nil {
			for _, check := range *requiredStatusChecks.Checks {
				checks = append(checks, check.Context)
			}
		}
		return checks, nil
	}
	return nil, nil
}

// GetEvergreenRulesetRules gets all Evergreen rules from rulesets as a list of
// strings. This may return duplicate rules.
func GetEvergreenRulesetRules(ctx context.Context, owner, repo, branch string) ([]string, error) {
	rules, err := GetRulesetRules(ctx, owner, repo, branch)
	if err != nil {
		return nil, err
	}

	return getRulesWithEvergreenPrefix(rules), nil
}

// GetRulesetRules returns all active rules from rulesets that apply to a branch
// as a list of strings. This may return duplicate rules.
func GetRulesetRules(ctx context.Context, owner, repo, branch string) ([]string, error) {
	caller := "GetRulesForBranch"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{})
	defer githubClient.Close()

	rules, resp, err := githubClient.Repositories.GetRulesForBranch(ctx, owner, repo, branch)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "getting rules for branch")
	}

	requiredStatusChecks := rules.RequiredStatusChecks
	if requiredStatusChecks == nil {
		return nil, nil
	}

	checks := []string{}
	for _, statusCheck := range requiredStatusChecks {
		if statusCheck == nil {
			continue
		}
		for _, check := range statusCheck.Parameters.RequiredStatusChecks {
			if check == nil {
				continue
			}
			checks = append(checks, check.Context)
		}
	}
	return checks, nil
}

func getCheckRunConclusion(status string) string {
	switch status {
	case evergreen.TaskSucceeded:
		return githubCheckRunSuccess
	case evergreen.TaskAborted:
		return githubCheckRunSkipped
	case evergreen.TaskFailed:
		return githubCheckRunActionRequired
	case evergreen.TaskTimedOut:
		return githubCheckRunTimedOut
	default:
		return githubCheckRunFailure
	}
}

func makeTaskLink(uiBase string, taskID string, execution int) string {
	return fmt.Sprintf("%s/task/%s/%d", uiBase, url.PathEscape(taskID), execution)
}

func makeCheckRunName(task *task.Task) string {
	return fmt.Sprintf("%s/%s", task.BuildVariantDisplayName, task.DisplayName)
}

// CreateCheckRun creates a checkRun and returns a Github CheckRun object
func CreateCheckRun(ctx context.Context, owner, repo, headSHA, uiBase string, task *task.Task, output *github.CheckRunOutput) (*github.CheckRun, error) {
	caller := "createCheckrun"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()

	opts := github.CreateCheckRunOptions{
		Name:        makeCheckRunName(task),
		HeadSHA:     headSHA,
		Output:      output,
		ExternalID:  utility.ToStringPtr(task.Id),
		StartedAt:   &github.Timestamp{Time: task.StartTime},
		CompletedAt: &github.Timestamp{Time: task.FinishTime},
		Status:      utility.ToStringPtr(githubCheckRunCompleted),
		Conclusion:  utility.ToStringPtr(getCheckRunConclusion(task.Status)),
		DetailsURL:  utility.ToStringPtr(makeTaskLink(uiBase, task.Id, task.Execution)),
	}

	checkRun, resp, err := githubClient.Checks.CreateCheckRun(ctx, owner, repo, opts)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "creating checkRun")
	}

	return checkRun, nil
}

// UpdateCheckRun updates a checkRun and returns a Github CheckRun object
// UpdateCheckRunOptions must specify a name for the check run
func UpdateCheckRun(ctx context.Context, owner, repo, uiBase string, checkRunID int64, task *task.Task, output *github.CheckRunOutput) (*github.CheckRun, error) {
	caller := "updateCheckrun"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	if output == nil {
		output = &github.CheckRunOutput{
			Title:   utility.ToStringPtr("Task restarted with no output"),
			Summary: utility.ToStringPtr(fmt.Sprintf("Task '%s' at execution %d was completed", task.DisplayName, task.Execution+1)),
		}
	}

	updateOpts := github.UpdateCheckRunOptions{
		Name:       makeCheckRunName(task),
		Status:     utility.ToStringPtr(githubCheckRunCompleted),
		Conclusion: utility.ToStringPtr(getCheckRunConclusion(task.Status)),
		DetailsURL: utility.ToStringPtr(makeTaskLink(uiBase, task.Id, task.Execution)),
		Output:     output,
	}

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()
	checkRun, resp, err := githubClient.Checks.UpdateCheckRun(ctx, owner, repo, checkRunID, updateOpts)

	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "updating checkRun")
	}

	return checkRun, nil
}

func ValidateCheckRunOutput(output *github.CheckRunOutput) error {
	if output == nil {
		return nil
	}

	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(output.Title == nil, "checkRun output has no title")
	summaryErrMsg := fmt.Sprintf("the checkRun output '%s' has no summary", utility.FromStringPtr(output.Title))
	catcher.NewWhen(output.Summary == nil, summaryErrMsg)

	for _, an := range output.Annotations {
		annotationErrorMessage := fmt.Sprintf("checkRun output '%s' specifies an annotation '%s' with no", utility.FromStringPtr(output.Title), utility.FromStringPtr(an.Title))

		catcher.NewWhen(an.Path == nil, fmt.Sprintf("%s path", annotationErrorMessage))
		invalidStart := an.StartLine == nil || utility.FromIntPtr(an.StartLine) < 1
		catcher.NewWhen(invalidStart, fmt.Sprintf("%s start line or a start line < 1", annotationErrorMessage))

		invalidEnd := an.EndLine == nil || utility.FromIntPtr(an.EndLine) < 1
		catcher.NewWhen(invalidEnd, fmt.Sprintf("%s end line or an end line < 1", annotationErrorMessage))

		catcher.NewWhen(an.AnnotationLevel == nil, fmt.Sprintf("%s annotation level", annotationErrorMessage))

		catcher.NewWhen(an.Message == nil, fmt.Sprintf("%s message", annotationErrorMessage))

		if an.EndColumn != nil || an.StartColumn != nil {
			if utility.FromIntPtr(an.StartLine) != utility.FromIntPtr(an.EndLine) {
				errMessage := fmt.Sprintf("The annotation '%s' in checkRun '%s' should not include a start or end column when start_line and end_line have different values", utility.FromStringPtr(an.Title), utility.FromStringPtr(output.Title))
				catcher.New(errMessage)
			}
		}
	}

	return catcher.Resolve()
}

// ListCheckRunCheckSuite returns a list of check run IDs for a given check suite
func ListCheckRunCheckSuite(ctx context.Context, owner, repo string, checkSuiteID int64) ([]int64, error) {
	caller := "ListCheckRunCheckSuite"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()
	listCheckRunsResult, resp, err := githubClient.Checks.ListCheckRunsCheckSuite(ctx, owner, repo, checkSuiteID, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "listing check suite")
	}
	checkRunIDs := []int64{}
	for _, checkRun := range listCheckRunsResult.CheckRuns {
		checkRunIDs = append(checkRunIDs, checkRun.GetID())
	}
	return checkRunIDs, nil
}

// GetCheckRun gets a check run by ID
func GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*github.CheckRun, error) {
	caller := "GetCheckRun"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	token, err := getInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting installation token")
	}

	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
	defer githubClient.Close()
	checkRun, resp, err := githubClient.Checks.GetCheckRun(ctx, owner, repo, checkRunID)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrapf(err, "getting check run %d", checkRunID)
	}
	return checkRun, nil
}
