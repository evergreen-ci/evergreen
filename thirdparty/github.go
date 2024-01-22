package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/cache"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/gregjones/httpcache"
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

	GithubInvestigation = "Github API Limit Investigation"
)

const (
	githubEndpointAttribute = "evergreen.github.endpoint"
	githubOwnerAttribute    = "evergreen.github.owner"
	githubRepoAttribute     = "evergreen.github.repo"
	githubRefAttribute      = "evergreen.github.ref"
	githubPathAttribute     = "evergreen.github.path"
	githubRetriesAttribute  = "evergreen.github.retries"
	githubCachedAttribute   = "evergreen.github.cached"
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
	"admin",
	"write",
}

const (
	GithubPRBlocked = "blocked"

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
)

var (
	githubTransport http.RoundTripper
	cacheTransport  *httpcache.Transport

	// TODO: (EVG-19966) Remove this error type.
	missingTokenError = errors.New("missing installation token")
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

func respFromCache(resp *http.Response) bool {
	// github.com/gregjones/httpcache adds the [X-From-Cache] header when the request is
	// fulfilled from the cache.
	//
	// [X-From-Cache]: https://pkg.go.dev/github.com/gregjones/httpcache#pkg-constants
	return resp.Header.Get(httpcache.XFromCache) != ""
}

// IsUnblockedGithubStatus returns true if the status is in the list of unblocked statuses
func IsUnblockedGithubStatus(status string) bool {
	return utility.StringSliceContains(UnblockedGithubStatuses, status)
}

// GithubPatch stores patch data for patches create from GitHub pull requests
type GithubPatch struct {
	PRNumber       int    `bson:"pr_number"`
	BaseOwner      string `bson:"base_owner"`
	BaseRepo       string `bson:"base_repo"`
	BaseBranch     string `bson:"base_branch"`
	HeadOwner      string `bson:"head_owner"`
	HeadRepo       string `bson:"head_repo"`
	HeadHash       string `bson:"head_hash"`
	Author         string `bson:"author"`
	AuthorUID      int    `bson:"author_uid"`
	MergeCommitSHA string `bson:"merge_commit_sha"`
	CommitTitle    string `bson:"commit_title"`
	CommitMessage  string `bson:"commit_message"`
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
	GithubPatchPRNumberKey       = bsonutil.MustHaveTag(GithubPatch{}, "PRNumber")
	GithubPatchBaseOwnerKey      = bsonutil.MustHaveTag(GithubPatch{}, "BaseOwner")
	GithubPatchBaseRepoKey       = bsonutil.MustHaveTag(GithubPatch{}, "BaseRepo")
	GithubPatchMergeCommitSHAKey = bsonutil.MustHaveTag(GithubPatch{}, "MergeCommitSHA")
	RepeatPatchIdNextPatchKey    = bsonutil.MustHaveTag(GithubPatch{}, "RepeatPatchIdNextPatch")
)

type retryConfig struct {
	retry    bool
	retry404 bool
}

func githubShouldRetry(caller string, config retryConfig) utility.HTTPRetryFunction {
	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		trace.SpanFromContext(req.Context()).SetAttributes(attribute.Int(githubRetriesAttribute, index))

		if !(config.retry || config.retry404) {
			return false
		}

		if index >= evergreen.GitHubMaxRetries {
			return false
		}

		url := req.URL.String()

		if err != nil {
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
		logGitHubRateLimit(limit)

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
func getGithubClient(token, caller string, config retryConfig) *github.Client {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClient",
		"caller":  caller,
	})

	// If the Environment is not nil that means we're running in the application and we have a connection
	// to the database. Otherwise we're running in the agent and we should use an in-memory cache.
	// We could stop casing on this if we were to stop calling out to GitHub from the agent.
	if evergreen.GetEnvironment() != nil {
		cacheTransport.Cache = &cache.DBCache{}
	} else {
		cacheTransport.Cache = httpcache.NewMemoryCache()
	}

	client := utility.SetupOauth2CustomHTTPRetryableClient(
		token,
		githubShouldRetry(caller, config),
		utility.RetryHTTPDelay(utility.RetryOptions{
			MaxAttempts: evergreen.GitHubMaxRetries,
			MinDelay:    evergreen.GitHubRetryMinDelay,
		}),
		utility.DefaultHttpClient(githubTransport),
	)

	return github.NewClient(client)
}

// getInstallationToken creates an installation token using Github app auth.
// If creating a token fails it will return the legacyToken.
func getInstallationToken(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting config")
	}

	token, err := settings.CreateInstallationToken(ctx, owner, repo, opts)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error creating token",
			"ticket":  "EVG-19966",
			"owner":   owner,
			"repo":    repo,
		}))
		return "", errors.Wrap(err, "creating token")
	}
	// TODO: (EVG-19966) Remove once CreateInstallationToken returns an error.
	if token == "" {
		return "", missingTokenError
	}

	return token, nil
}

func getInstallationTokenWithDefaultOwnerRepo(ctx context.Context, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting evergreen settings")
	}
	token, err := settings.CreateInstallationTokenWithDefaultOwnerRepo(ctx, opts)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error creating default token",
			"ticket":  "EVG-19966",
		}))
		return "", errors.Wrap(err, "creating default installation token")
	}
	// TODO: (EVG-19966) Remove once CreateInstallationTokenWithDefaultOwnerRepo returns an error.
	if token == "" {
		return "", missingTokenError
	}

	return token, nil
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, token, owner, repo, ref string, until time.Time, commitPage int) ([]*github.RepositoryCommit, int, error) {
	commits, nextPage, err := getCommits(ctx, "", owner, repo, ref, until, commitPage)
	if err == nil {
		return commits, nextPage, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get commits from GitHub",
		"caller":  "GetGithubCommits",
		"owner":   owner,
		"repo":    repo,
		"ref":     ref,
	}))

	return getCommits(ctx, token, owner, repo, ref, until, commitPage)
}

func getCommits(ctx context.Context, token, owner, repo, ref string, until time.Time, commitPage int) ([]*github.RepositoryCommit, int, error) {
	caller := "GetGithubCommits"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, 0, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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
func GetGithubFile(ctx context.Context, token, owner, repo, path, ref string) (*github.RepositoryContent, error) {
	if path == "" {
		return nil, errors.New("remote repository path cannot be empty")
	}

	content, err := getFile(ctx, "", owner, repo, path, ref)
	if err == nil {
		return content, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get a file from GitHub",
		"caller":  "GetGithubFile",
		"owner":   owner,
		"repo":    repo,
		"path":    path,
		"ref":     ref,
	}))

	return getFile(ctx, token, owner, repo, path, ref)
}

func getFile(ctx context.Context, token, owner, repo, path, ref string) (*github.RepositoryContent, error) {
	caller := "GetGithubFile"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
		attribute.String(githubPathAttribute, path),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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
			return nil, FileNotFoundError{filepath: path}
		}
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from github for '%s/%s' for '%s': %v", owner, repo, path, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}

	if file == nil || file.Content == nil {
		return nil, APIRequestError{Message: "file is nil"}
	}

	return file, nil
}

// SendPendingStatusToGithub sends a pending status to a Github PR patch
// associated with a given version.
func SendPendingStatusToGithub(ctx context.Context, input SendGithubStatusInput, urlBase string) error {
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
		URL:         fmt.Sprintf("%s/version/%s?redirect_spruce_users=true", urlBase, input.VersionId),
		Context:     input.Context,
		State:       message.GithubStatePending,
		Description: input.Desc,
	}

	sender, err := env.GetGitHubSender(input.Owner, input.Repo)
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
func GetGithubMergeBaseRevision(ctx context.Context, token, owner, repo, baseRevision, currentCommitHash string) (string, error) {
	mergeBase, err := getMergeBaseRevision(ctx, "", owner, repo, baseRevision, currentCommitHash)
	if err == nil {
		return mergeBase, nil
	}
	grip.Debug(message.WrapError(err, message.Fields{
		"ticket":              "EVG-19966",
		"message":             "failed to get merge-base from GitHub",
		"caller":              "GetGithubMergeBaseRevision",
		"owner":               owner,
		"repo":                repo,
		"base_revision":       baseRevision,
		"current_commit_hash": currentCommitHash,
	}))

	return getMergeBaseRevision(ctx, token, owner, repo, baseRevision, currentCommitHash)
}

func getMergeBaseRevision(ctx context.Context, token, owner, repo, baseRevision, currentCommitHash string) (string, error) {
	caller := "GetGithubMergeBaseRevision"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return "", errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

	compare, resp, err := githubClient.Repositories.CompareCommits(ctx,
		owner, repo, baseRevision, currentCommitHash, nil)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return "", parseGithubErrorResponse(resp)
		}
	} else {
		apiErr := errors.Errorf("nil response from merge base commit response for '%s/%s'@%s..%s: %v", owner, repo, baseRevision, currentCommitHash, err)
		grip.Error(message.WrapError(apiErr, message.Fields{
			"message":             "failed to compare commits to find merge base commit",
			"op":                  "GetGithubMergeBaseRevision",
			"github_error":        fmt.Sprint(err),
			"repo":                repo,
			"base_revision":       baseRevision,
			"current_commit_hash": currentCommitHash,
		}))
		return "", APIResponseError{apiErr.Error()}
	}

	if compare == nil || compare.MergeBaseCommit == nil || compare.MergeBaseCommit.SHA == nil {
		return "", APIRequestError{Message: "missing data from GitHub compare response"}
	}

	return *compare.MergeBaseCommit.SHA, nil
}

func GetCommitEvent(ctx context.Context, token, owner, repo, githash string) (*github.RepositoryCommit, error) {
	event, err := commitEvent(ctx, "", owner, repo, githash)
	if err == nil {
		return event, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get commit event from GitHub",
		"caller":  "GetCommitEvent",
		"owner":   owner,
		"repo":    repo,
	}))

	return commitEvent(ctx, token, owner, repo, githash)
}

func commitEvent(ctx context.Context, token, owner, repo, githash string) (*github.RepositoryCommit, error) {
	caller := "GetCommitEvent"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, githash),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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

	return commit, nil
}

// GetCommitDiff gets the diff of the specified commit via an API call to GitHub
func GetCommitDiff(ctx context.Context, token, owner, repo, sha string) (string, error) {
	diff, err := commitDiff(ctx, "", owner, repo, sha)
	if err == nil {
		return diff, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get commit diff from GitHub",
		"caller":  "GetCommitDiff",
		"owner":   owner,
		"repo":    repo,
		"sha":     sha,
	}))

	return commitDiff(ctx, token, owner, repo, sha)
}

func commitDiff(ctx context.Context, token, owner, repo, sha string) (string, error) {
	caller := "GetCommitDiff"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return "", errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

	commit, resp, err := githubClient.Repositories.GetCommitRaw(ctx, owner, repo, sha, github.RawOptions{Type: github.Diff})
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
		if err != nil {
			return "", parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from '%s/%s': sha: '%s': %v", owner, repo, sha, err)
		grip.Error(message.Fields{
			"message": errMsg,
			"owner":   owner,
			"repo":    repo,
			"sha":     sha,
		})
		return "", APIResponseError{errMsg}
	}

	return commit, nil
}

// GetBranchEvent gets the head of the a given branch via an API call to GitHub
func GetBranchEvent(ctx context.Context, token, owner, repo, branch string) (*github.Branch, error) {
	event, err := branchEvent(ctx, "", owner, repo, branch)
	if err == nil {
		return event, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get branch event from GitHub",
		"caller":  "GetBranchEvent",
		"owner":   owner,
		"repo":    repo,
		"branch":  branch,
	}))

	return branchEvent(ctx, token, owner, repo, branch)
}

func branchEvent(ctx context.Context, token, owner, repo, branch string) (*github.Branch, error) {
	caller := "GetBranchEvent"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", owner, repo, branch)

	branchEvent, resp, err := githubClient.Repositories.GetBranch(ctx, owner, repo, branch, false)
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
func githubRequest(ctx context.Context, method string, url string, oauthToken string, data interface{}) (*http.Response, error) {
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
func tryGithubPost(ctx context.Context, url string, oauthToken string, data interface{}) (resp *http.Response, err error) {
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
		logGitHubRateLimit(parseGithubRateLimit(resp.Header))

		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: evergreen.GitHubMaxRetries,
		MinDelay:    evergreen.GitHubRetryMinDelay,
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

func logGitHubRateLimit(limit github.Rate) {
	if limit.Limit == 0 {
		grip.Error(message.Fields{
			"message": "GitHub API rate limit",
			"error":   "can't parse rate limit",
		})
	} else if limit.Limit == 60 {
		// TODO EVG-19966: remove manual log remover
		return
	} else {
		grip.Info(message.Fields{
			"message":           "GitHub API rate limit",
			"remaining":         limit.Remaining,
			"limit":             limit.Limit,
			"reset":             limit.Reset,
			"minutes_remaining": time.Until(limit.Reset.Time).Minutes(),
			"percentage":        float32(limit.Remaining) / float32(limit.Limit),
		})
	}
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
func GetTaggedCommitFromGithub(ctx context.Context, token, owner, repo, tag string) (string, error) {
	sha, err := taggedCommit(ctx, "", owner, repo, tag)
	if err == nil {
		return sha, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get tagged commit from GitHub",
		"caller":  "GetTaggedCommitFromGithub",
		"owner":   owner,
		"repo":    repo,
		"tag":     tag,
	}))

	return taggedCommit(ctx, token, owner, repo, tag)
}

func taggedCommit(ctx context.Context, token, owner, repo, tag string) (string, error) {
	caller := "GetTaggedCommitFromGithub"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, tag),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return "", errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})
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
		annotatedTag, err := getObjectTag(ctx, token, owner, repo, tagSha)
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

func getObjectTag(ctx context.Context, token, owner, repo, sha string) (*github.Tag, error) {
	caller := "getObjectTag"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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

func IsUserInGithubTeam(ctx context.Context, teams []string, org, user, token, owner, repo string) bool {
	inTeam, err := userInTeam(ctx, "", teams, org, user, owner, repo)
	if err == nil {
		return inTeam
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get team membership from GitHub",
		"caller":  "IsUserInGithubTeam",
		"org":     org,
		"owner":   owner,
		"repo":    repo,
		"teams":   teams,
		"user":    user,
	}))

	inTeam, _ = userInTeam(ctx, token, teams, org, user, owner, repo)
	return inTeam
}

func userInTeam(ctx context.Context, token string, teams []string, org, user, owner, repo string) (bool, error) {
	caller := "IsUserInGithubTeam"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return false, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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

// CheckGithubAPILimit queries Github for the number of API requests remaining
func CheckGithubAPILimit(ctx context.Context, token string) (int64, error) {
	limit, err := apiLimit(ctx, "")
	if err == nil {
		return limit, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get API limit from GitHub",
		"caller":  "CheckGithubAPILimit",
	}))

	return apiLimit(ctx, token)
}

func apiLimit(ctx context.Context, token string) (int64, error) {
	caller := "CheckGithubAPILimit"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
		if err != nil {
			return 0, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

	limits, resp, err := githubClient.RateLimits(ctx)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		grip.Errorf("github GET rate limit failed: %+v", err)
		return 0, err
	}

	if limits.Core == nil {
		return 0, errors.New("nil github limits")
	}
	if limits.Core.Remaining < 0 {
		return int64(0), nil
	}

	return int64(limits.Core.Remaining), nil
}

// GetGithubUser fetches the github user with the given login name
func GetGithubUser(ctx context.Context, token, loginName string) (*github.User, error) {
	user, err := getUser(ctx, "", loginName)
	if err == nil {
		return user, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get user from GitHub",
		"caller":  "GetGithubUser",
		"user":    loginName,
	}))

	return getUser(ctx, token, loginName)
}

func getUser(ctx context.Context, token, loginName string) (*github.User, error) {
	caller := "GetGithubUser"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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
func GithubUserInOrganization(ctx context.Context, token, requiredOrganization, username string) (bool, error) {
	inOrg, err := userInOrganization(ctx, "", requiredOrganization, username)
	if err == nil {
		return inOrg, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to check user in org from GitHub",
		"caller":  "GithubUserInOrganization",
		"org":     requiredOrganization,
		"user":    username,
	}))

	return userInOrganization(ctx, token, requiredOrganization, username)
}

func userInOrganization(ctx context.Context, token, requiredOrganization, username string) (bool, error) {
	caller := "GithubUserInOrganization"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
		if err != nil {
			return false, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

	isMember, resp, err := githubClient.Organizations.IsMember(context.Background(), requiredOrganization, username)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	return isMember, err
}

// AppAuthorizedForOrg returns true if the given app name exists in the org's installation list,
// and has permission to write to pull requests. Returns an error if the app name exists but doesn't have permission.
func AppAuthorizedForOrg(ctx context.Context, token, requiredOrganization, name string) (bool, error) {
	authorized, err := authorizedForOrg(ctx, "", requiredOrganization, name)
	if err == nil {
		return authorized, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to check app in org from GitHub",
		"caller":  "AppAuthorizedForOrg",
		"org":     requiredOrganization,
		"name":    name,
	}))

	return authorizedForOrg(ctx, token, requiredOrganization, name)
}

func authorizedForOrg(ctx context.Context, token, requiredOrganization, name string) (bool, error) {
	caller := "AppAuthorizedForOrg"
	const botSuffix = "[bot]"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationTokenWithDefaultOwnerRepo(ctx, nil)
		if err != nil {
			return false, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry: true})

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

		if resp.NextPage > 0 {
			opts.Page = resp.NextPage
		} else {
			break
		}
	}

	return false, nil
}

// GitHubUserHasWritePermission returns true if the given user has write permission for the repo.
// Returns an error if the user isn't found or the token isn't authed for this repo.
func GitHubUserHasWritePermission(ctx context.Context, token, owner, repo, username string) (bool, error) {
	level, err := userHasWritePermission(ctx, "", owner, repo, username)
	if err == nil {
		return level, nil
	}
	grip.Debug(message.WrapError(err, message.Fields{
		"ticket":   "EVG-19966",
		"message":  "failed to check user permission level from GitHub",
		"caller":   "GitHubUserPermissionLevel",
		"owner":    owner,
		"repo":     repo,
		"username": username,
	}))

	return userHasWritePermission(ctx, token, owner, repo, username)
}

func userHasWritePermission(ctx context.Context, token, owner, repo, username string) (bool, error) {
	caller := "userHasWritePermission"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return false, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})

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

// GetPullRequestMergeBase returns the merge base hash for the given PR.
// This function will retry up to 5 times, regardless of error response (unless
// error is the result of hitting an api limit)
func GetPullRequestMergeBase(ctx context.Context, token string, data GithubPatch) (string, error) {
	mergeBase, err := getPRMergeBase(ctx, "", data)
	if err == nil {
		return mergeBase, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get PR merge base from GitHub",
		"caller":  "GetPullRequestMergeBase",
		"owner":   data.BaseOwner,
		"repo":    data.BaseRepo,
	}))

	return getPRMergeBase(ctx, token, data)
}

func getPRMergeBase(ctx context.Context, token string, data GithubPatch) (string, error) {
	caller := "GetPullRequestMergeBase"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, data.BaseOwner),
		attribute.String(githubRepoAttribute, data.BaseRepo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, data.BaseOwner, data.BaseRepo, nil)
		if err != nil {
			return "", errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})

	commits, resp, err := githubClient.PullRequests.ListCommits(ctx, data.BaseOwner, data.BaseRepo, data.PRNumber, nil)
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

	commit, err := getCommit(ctx, token, data.BaseOwner, data.BaseRepo, *commits[0].SHA)
	if err != nil {
		return "", errors.Wrapf(err, "getting commit on %s/%s with SHA '%s'", data.BaseOwner, data.BaseRepo, *commits[0].SHA)
	}
	if len(commit.Parents) == 0 {
		return "", errors.New("can't find pull request branch point")
	}
	if commit.Parents[0].GetSHA() == "" {
		return "", errors.New("parent hash is missing")
	}

	return commit.Parents[0].GetSHA(), nil
}

func getCommit(ctx context.Context, token, owner, repo, sha string) (*github.RepositoryCommit, error) {
	caller := "getCommit"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})

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

func GetGithubPullRequest(ctx context.Context, token, baseOwner, baseRepo string, prNumber int) (*github.PullRequest, error) {
	pr, err := getPullRequest(ctx, "", baseOwner, baseRepo, prNumber)
	if err == nil {
		return pr, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":         "EVG-19966",
		"message":        "failed to get PR from GitHub",
		"caller":         "GetGithubPullRequest",
		"owner":          baseOwner,
		"repo":           baseRepo,
		"pr_num":         prNumber,
		"token is empty": token == "",
	}))

	return getPullRequest(ctx, token, baseOwner, baseRepo, prNumber)
}

func getPullRequest(ctx context.Context, token, baseOwner, baseRepo string, prNumber int) (*github.PullRequest, error) {
	caller := "GetGithubPullRequest"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, baseOwner),
		attribute.String(githubRepoAttribute, baseRepo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, baseOwner, baseRepo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})

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

// GetGithubPullRequestDiff downloads a diff from a Github Pull Request diff
func GetGithubPullRequestDiff(ctx context.Context, token string, gh GithubPatch) (string, []Summary, error) {
	diff, summary, err := pullRequestDiff(ctx, "", gh)
	if err == nil {
		return diff, summary, nil
	}
	// TODO: (EVG-19966) Remove logging.
	grip.DebugWhen(!errors.Is(err, missingTokenError), message.WrapError(err, message.Fields{
		"ticket":  "EVG-19966",
		"message": "failed to get PR diff from GitHub",
		"caller":  "GetGithubPullRequestDiff",
		"owner":   gh.BaseOwner,
		"repo":    gh.BaseRepo,
		"pr_num":  gh.PRNumber,
	}))

	return pullRequestDiff(ctx, token, gh)
}

func pullRequestDiff(ctx context.Context, token string, gh GithubPatch) (string, []Summary, error) {
	caller := "GetGithubPullRequestDiff"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, gh.BaseOwner),
		attribute.String(githubRepoAttribute, gh.BaseRepo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, gh.BaseOwner, gh.BaseRepo, nil)
		if err != nil {
			return "", nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{retry404: true})

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

func ValidatePR(pr *github.PullRequest) error {
	if pr == nil {
		return errors.New("No PR provided")
	}

	catcher := grip.NewSimpleCatcher()
	if pr.GetMergeCommitSHA() == "" {
		catcher.Add(errors.New("no merge commit SHA"))
	}
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

func SendCommitQueueGithubStatus(ctx context.Context, env evergreen.Environment, pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	owner := utility.FromStringPtr(pr.Base.Repo.Owner.Login)
	repo := utility.FromStringPtr(pr.Base.Repo.Name)

	sender, err := env.GetGitHubSender(owner, repo)
	if err != nil {
		return errors.Wrap(err, "getting github status sender")
	}

	var url string
	if versionID != "" {
		uiConfig := evergreen.UIConfig{}
		if err := uiConfig.Get(ctx); err == nil {
			url = fmt.Sprintf("%s/version/%s?redirect_spruce_users=true", uiConfig.Url, versionID)
		}
	}

	msg := message.GithubStatus{
		Owner:       owner,
		Repo:        repo,
		Ref:         utility.FromStringPtr(pr.Head.SHA),
		Context:     commitqueue.GithubContext,
		State:       state,
		Description: description,
		URL:         url,
	}

	c := message.NewGithubStatusMessageWithRepo(level.Notice, msg)
	sender.Send(c)
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called github status send",
		"caller":  "commit queue github status",
	})
	return nil
}

// GetMergeablePullRequest gets the pull request and returns if the PR is valid and mergeable.
func GetMergeablePullRequest(ctx context.Context, issue int, githubToken, owner, repo string) (*github.PullRequest, error) {
	pr, err := GetGithubPullRequest(ctx, githubToken, owner, repo, issue)
	if err != nil {
		return nil, errors.Wrap(err, "can't get PR from GitHub")
	}

	if err = ValidatePR(pr); err != nil {
		return nil, errors.Wrap(err, "GitHub returned an incomplete PR")
	}

	if !utility.FromBoolTPtr(pr.Mergeable) {
		return pr, errors.New("PR is not mergeable")
	}

	return pr, nil
}

// MergePullRequest attempts to merge the given pull request. If commits are merged one after another, Github may
// not have updated that this can be merged, so we allow retries.
func MergePullRequest(ctx context.Context, token, appToken, owner, repo, commitMessage string, prNum int, mergeOpts *github.PullRequestOptions) error {
	err := mergePR(ctx, appToken, owner, repo, commitMessage, prNum, mergeOpts)
	if err == nil {
		return nil
	}
	grip.Debug(message.WrapError(err, message.Fields{
		"ticket":    "EVG-19966",
		"message":   "failed to merge PR on GitHub",
		"caller":    "MergePullRequest",
		"owner":     owner,
		"repo":      repo,
		"pr_number": prNum,
		"opts":      mergeOpts,
	}))

	return mergePR(ctx, token, owner, repo, commitMessage, prNum, mergeOpts)
}

func mergePR(ctx context.Context, token, owner, repo, commitMessage string, prNum int, mergeOpts *github.PullRequestOptions) error {
	caller := "MergePullRequest"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	githubClient := getGithubClient(token, caller, retryConfig{})
	res, resp, err := githubClient.PullRequests.Merge(ctx, owner, repo,
		prNum, commitMessage, mergeOpts)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return errors.Wrap(err, "accessing GitHub merge API")
	}

	if !res.GetMerged() {
		return errors.Errorf("GitHub refused to merge PR '%s/%s:%d': '%s'", owner, repo, prNum, res.GetMessage())
	}
	return nil
}

// PostCommentToPullRequest posts the given comment to the associated PR.
func PostCommentToPullRequest(ctx context.Context, token, owner, repo string, prNum int, comment string) error {
	err := postComment(ctx, "", owner, repo, prNum, comment)
	if err == nil {
		return nil
	}
	grip.Debug(message.WrapError(err, message.Fields{
		"ticket":    "EVG-19966",
		"message":   "failed to comment to PR on GitHub",
		"caller":    "PostCommentToPullRequest",
		"owner":     owner,
		"repo":      repo,
		"pr_number": prNum,
	}))

	return postComment(ctx, token, owner, repo, prNum, comment)
}

func postComment(ctx context.Context, token, owner, repo string, prNum int, comment string) error {
	caller := "PostCommentToPullRequest"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{})

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
	if resp.StatusCode != http.StatusCreated || respComment == nil || respComment.ID == nil {
		return errors.New("unexpected data from GitHub")
	}
	return nil
}

// GetEvergreenBranchProtectionRules gets all Evergreen branch protection checks as a list of strings.
func GetEvergreenBranchProtectionRules(ctx context.Context, token, owner, repo, branch string) ([]string, error) {
	branchProtectionRules, err := GetBranchProtectionRules(ctx, token, owner, repo, branch)
	if err != nil {
		return nil, err
	}
	return getRulesWithEvergreenPrefix(branchProtectionRules), nil
}

func getRulesWithEvergreenPrefix(rules []string) []string {
	rulesWithEvergreenPrefix := []string{}
	for _, rule := range rules {
		if strings.HasPrefix(rule, "evergreen") {
			rulesWithEvergreenPrefix = append(rulesWithEvergreenPrefix, rule)
		}
	}
	return rulesWithEvergreenPrefix
}

// GetBranchProtectionRules gets all branch protection checks as a list of strings.
func GetBranchProtectionRules(ctx context.Context, token, owner, repo, branch string) ([]string, error) {
	caller := "GetBranchProtection"
	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(
		attribute.String(githubEndpointAttribute, caller),
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	))
	defer span.End()

	if token == "" {
		var err error
		token, err = getInstallationToken(ctx, owner, repo, nil)
		if err != nil {
			return nil, errors.Wrap(err, "getting installation token")
		}
	}
	githubClient := getGithubClient(token, caller, retryConfig{})

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
		for _, check := range requiredStatusChecks.Checks {
			checks = append(checks, check.Context)
		}
		return checks, nil
	}
	return nil, nil
}

// createCheckrun creates a checkRun and returns a Github CheckRun object
func CreateCheckrun(ctx context.Context, owner, repo, name, headSHA string, output *github.CheckRunOutput) (*github.CheckRun, error) {
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

	opts := github.CreateCheckRunOptions{
		Output:  output,
		Name:    name,
		HeadSHA: headSHA,
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

// UpdateCheckrun updates a checkRun and returns a Github CheckRun object
func UpdateCheckrun(ctx context.Context, owner, repo, name string, checkRunID int64, output *github.CheckRunOutput) (*github.CheckRun, error) {
	caller := "updateCheckrun"
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

	opts := github.UpdateCheckRunOptions{
		Output: output,
		Name:   name,
	}

	checkRun, resp, err := githubClient.Checks.UpdateCheckRun(ctx, owner, repo, checkRunID, opts)
	if resp != nil {
		defer resp.Body.Close()
		span.SetAttributes(attribute.Bool(githubCachedAttribute, respFromCache(resp.Response)))
	}
	if err != nil {
		return nil, errors.Wrap(err, "updating checkRun")
	}

	return checkRun, nil
}

func ValidateCheckRun(checkRun *github.CheckRunOutput) error {
	if checkRun == nil {
		return errors.New("checkRun Output is nil")
	}

	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(checkRun.Title == nil, "checkRun has no title")
	summaryErrMsg := fmt.Sprintf("the checkRun '%s' has no summary", utility.FromStringPtr(checkRun.Title))
	catcher.NewWhen(checkRun.Summary == nil, summaryErrMsg)

	for _, an := range checkRun.Annotations {
		annotationErrorMessage := fmt.Sprintf("checkRun '%s' specifies an annotation '%s' with no ", utility.FromStringPtr(checkRun.Title), utility.FromStringPtr(an.Title))

		catcher.NewWhen(an.Path == nil, annotationErrorMessage+"path")
		invalidStart := an.StartLine == nil || utility.FromIntPtr(an.StartLine) < 1
		catcher.NewWhen(invalidStart, annotationErrorMessage+"start line or a start line < 1")

		invalidEnd := an.EndLine == nil || utility.FromIntPtr(an.EndLine) < 1
		catcher.NewWhen(invalidEnd, annotationErrorMessage+"end line or an end line < 1")

		catcher.NewWhen(an.AnnotationLevel == nil, annotationErrorMessage+"annotation level")

		catcher.NewWhen(an.Message == nil, annotationErrorMessage+"message")

		if an.EndColumn != nil || an.StartColumn != nil {
			if utility.FromIntPtr(an.StartLine) != utility.FromIntPtr(an.EndLine) {
				errMessage := fmt.Sprintf("The annotation '%s' in checkRun '%s' should not include a start or end column when start_line and end_line have different values", utility.FromStringPtr(an.Title), utility.FromStringPtr(checkRun.Title))
				catcher.New(errMessage)
			}
		}
	}

	return catcher.Resolve()
}
