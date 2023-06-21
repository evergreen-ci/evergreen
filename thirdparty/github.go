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
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
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
	numGithubAttempts   = 3
	githubRetryMinDelay = time.Second
	githubAccessURL     = "https://github.com/login/oauth/access_token"
	githubHookURL       = "%s/rest/v2/hooks/github"

	Github502Error   = "502 Server Error"
	commitObjectType = "commit"
	tagObjectType    = "tag"
	githubWrite      = "write"

	GithubInvestigation = "Github API Limit Investigation"

	// TODO EVG-20103: move this to admin settings
	defaultOwner = "evergreen-ci"
	defaultRepo  = "commit-queue-sandbox"
)

const (
	githubEndpointAttribute = "evergreen.github.endpoint"
	githubOwnerAttribute    = "evergreen.github.owner"
	githubRepoAttribute     = "evergreen.github.repo"
	githubRefAttribute      = "evergreen.github.ref"
	githubPathAttribute     = "evergreen.github.path"
	githubRetriesAttribute  = "evergreen.github.retries"
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
}

// GithubMergeGroup stores patch data for patches created from GitHub merge groups
type GithubMergeGroup struct {
	Org        string `bson:"org"`
	Repo       string `bson:"repo"`
	BaseBranch string `bson:"base_branch"` // BaseBranch is what GitHub merges to
	HeadBranch string `bson:"head_branch"` // HeadBranch is the merge group's gh-readonly-queue branch
	HeadSHA    string `bson:"head_sha"`
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

		if index >= numGithubAttempts {
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

// getGithubClient returns an *http.Client that will retry according to supplied retryConfig.
// Also creates a span tracking the lifespan of the client.
// Defer the returned function to release the client to the pool and close the span.
func getGithubClient(ctx context.Context, token, caller string, config retryConfig, attributes []attribute.KeyValue) (context.Context, *http.Client, func()) {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClient",
		"caller":  caller,
	})

	ctx, span := tracer.Start(ctx, caller, trace.WithAttributes(append(attributes, attribute.String(githubEndpointAttribute, caller))...))

	client := utility.GetHTTPClient()
	originalTransport := client.Transport
	client.Transport = otelhttp.NewTransport(client.Transport)

	return ctx,
		utility.SetupOauth2CustomHTTPRetryableClient(
			token,
			githubShouldRetry(caller, config),
			utility.RetryHTTPDelay(utility.RetryOptions{
				MaxAttempts: numGithubAttempts,
				MinDelay:    githubRetryMinDelay,
			}),
			client,
		), func() {
			client.Transport = originalTransport
			utility.PutHTTPClient(client)

			span.End()
		}
}

// GetInstallationToken creates an installation token using Github app auth.
func GetInstallationToken(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		return "", errors.Wrap(err, "getting evergreen settings")
	}
	token, err := settings.CreateInstallationToken(ctx, owner, repo, opts)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error creating token",
			"ticket":  "EVG-19966",
			"owner":   owner,
			"repo":    repo,
		}))
		return "", errors.Wrap(err, "creating installation token")
	}
	return token, nil
}

func getInstallationTokenWithoutOwnerRepo(ctx context.Context) (string, error) {
	return GetInstallationToken(ctx, defaultOwner, defaultRepo, nil)
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, token, owner, repo, ref string, until time.Time, commitPage int) ([]*github.RepositoryCommit, int, error) {
	options := github.CommitsListOptions{
		SHA: ref,
		ListOptions: github.ListOptions{
			Page: commitPage,
		},
	}
	if !utility.IsZeroTime(until) {
		options.Until = until
	}

	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubCommits", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, ref),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &options)
		if resp != nil {
			defer resp.Body.Close()
			if err == nil {
				return commits, resp.NextPage, nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get commits from GitHub",
			"caller":  "GetGithubCommits",
			"owner":   owner,
			"repo":    repo,
			"options": options,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubCommits", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	commits, newResp, err := client.Repositories.ListCommits(ctx, owner, repo, &options)
	if newResp != nil {
		defer newResp.Body.Close()
		if err != nil {
			return nil, 0, parseGithubErrorResponse(newResp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from query for commits in '%s/%s' ref %s : %v", owner, repo, ref, err)
		grip.Error(errMsg)
		return nil, 0, APIResponseError{errMsg}
	}

	return commits, newResp.NextPage, nil
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
	var opt *github.RepositoryContentGetOptions
	if len(ref) != 0 {
		opt = &github.RepositoryContentGetOptions{
			Ref: ref,
		}
	}

	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubFile", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, ref),
			attribute.String(githubPathAttribute, path),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		file, _, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, opt)
		if resp != nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound || file == nil || file.Content == nil {
				return nil, APIRequestError{Message: "file is nil"}
			}
			if err == nil {
				return file, nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get file from GitHub",
			"caller":  "GetGithubFile",
			"owner":   owner,
			"repo":    repo,
			"path":    path,
			"options": opt,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, httpLegacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubFile", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, ref),
		attribute.String(githubPathAttribute, path),
	})
	defer putLegacyClient()
	client := github.NewClient(httpLegacyClient)

	file, _, newResp, err := client.Repositories.GetContents(ctx, owner, repo, path, opt)
	if newResp != nil {
		defer newResp.Body.Close()
		if newResp.StatusCode == http.StatusNotFound {
			return nil, FileNotFoundError{filepath: path}
		}
		if err != nil {
			return nil, parseGithubErrorResponse(newResp)
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
func SendPendingStatusToGithub(input SendGithubStatusInput, urlBase string) error {
	flags, err := evergreen.GetServiceFlags()
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
		if err = uiConfig.Get(env); err != nil {
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
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
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
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {

		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubMergeBaseRevision", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		compare, resp, err := client.Repositories.CompareCommits(ctx, owner, repo, baseRevision, currentCommitHash, nil)
		if resp != nil {
			defer resp.Body.Close()
			if compare == nil || compare.MergeBaseCommit == nil || compare.MergeBaseCommit.SHA == nil {
				return "", APIRequestError{Message: "missing data from GitHub compare response"}
			}
			if err == nil {
				return compare.GetMergeBaseCommit().GetSHA(), nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get github merge base revision",
			"caller":  "GetGithubMergeBaseRevision",
			"owner":   owner,
			"repo":    repo,
			"base":    baseRevision,
			"current": currentCommitHash,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubMergeBaseRevision", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	compare, newResp, err := client.Repositories.CompareCommits(ctx, owner, repo, baseRevision, currentCommitHash, nil)
	if newResp != nil {
		defer newResp.Body.Close()
		if err != nil {
			return "", parseGithubErrorResponse(newResp)
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
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetCommitEvent", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, githash),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		commit, resp, err := client.Repositories.GetCommit(ctx, owner, repo, githash, nil)
		if resp != nil {
			defer resp.Body.Close()
			if err == nil && commit != nil {
				return commit, nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get github commit",
			"caller":  "GetCommitEvent",
			"owner":   owner,
			"repo":    repo,
			"commit":  githash,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetCommitEvent", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, githash),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    owner + "/" + repo,
	})

	commit, newResp, err := client.Repositories.GetCommit(ctx, owner, repo, githash, nil)
	if newResp != nil {
		defer newResp.Body.Close()
		if err != nil {
			return nil, parseGithubErrorResponse(newResp)
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
		"size":      newResp.ContentLength,
		"status":    newResp.Status,
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
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetCommitDiff", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, sha),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		commit, resp, err := client.Repositories.GetCommitRaw(ctx, owner, repo, sha, github.RawOptions{Type: github.Diff})
		if resp != nil {
			defer resp.Body.Close()
			if err == nil {
				return commit, nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get commit diff",
			"caller":  "GetCommitDiff",
			"owner":   owner,
			"repo":    repo,
			"commit":  sha,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetCommitDiff", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, sha),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	commit, newResp, err := client.Repositories.GetCommitRaw(ctx, owner, repo, sha, github.RawOptions{Type: github.Diff})
	if newResp != nil {
		defer newResp.Body.Close()
		if err != nil {
			return "", parseGithubErrorResponse(newResp)
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
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetBranchEvent", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, branch),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		branchEvent, resp, err := client.Repositories.GetBranch(ctx, owner, repo, branch, false)
		if resp != nil {
			defer resp.Body.Close()
			if err == nil {
				return branchEvent, nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get github branch",
			"caller":  "GetBranchEvent",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetBranchEvent", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, branch),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", owner, repo, branch)

	branchEvent, newResp, err := client.Repositories.GetBranch(ctx, owner, repo, branch, false)
	if newResp != nil {
		defer newResp.Body.Close()
		if err != nil {
			return nil, parseGithubErrorResponse(newResp)
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
		MaxAttempts: numGithubAttempts,
		MinDelay:    githubRetryMinDelay,
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
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetTaggedCommitFromGithub", retryConfig{retry: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
			attribute.String(githubRefAttribute, tag),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, nil)
		if resp != nil {
			defer resp.Body.Close()
		}

		for _, t := range tags {
			if t.GetName() == tag {
				return t.GetCommit().GetSHA(), nil
			}
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get tagged commit from github",
			"caller":  "GetTaggedCommitFromGithub",
			"owner":   owner,
			"repo":    repo,
			"resp":    resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetTaggedCommitFromGithub", retryConfig{retry: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
		attribute.String(githubRefAttribute, tag),
	})
	defer putLegacyClient()

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs/tags/%s", owner, repo, tag)
	newResp, err := legacyClient.Get(url)
	if newResp != nil {
		defer newResp.Body.Close()
	}
	if err != nil {
		return "", errors.Wrap(err, "failed to get tag information from GitHub")
	}
	if newResp == nil {
		return "", errors.New("invalid github response")
	}

	respBody, err := io.ReadAll(newResp.Body)
	if err != nil {
		return "", ResponseReadError{err.Error()}
	}
	tagResp := github.Tag{}
	if err = json.Unmarshal(respBody, &tagResp); err != nil {
		return "", APIUnmarshalError{string(respBody), err.Error()}
	}

	var sha string
	var annotatedTagResp *github.Tag
	tagSha := tagResp.GetObject().GetSHA()
	switch tagResp.GetObject().GetType() {
	case commitObjectType:
		// lightweight tags are pointers to the commit itself
		sha = tagSha
	case tagObjectType:
		githubClient := github.NewClient(legacyClient)
		annotatedTagResp, _, err = githubClient.Git.GetTag(ctx, owner, repo, tagSha)
		if err != nil {
			return "", errors.Wrapf(err, "error getting tag '%s' with SHA '%s'", tag, tagSha)
		}
		sha = annotatedTagResp.GetObject().GetSHA()
	default:
		return "", errors.Errorf("unrecognized object type '%s'", tagResp.GetObject().GetType())
	}

	if tagSha == "" {
		return "", errors.New("empty SHA from GitHub")
	}

	return sha, nil
}

func IsUserInGithubTeam(ctx context.Context, teams []string, org, user, oauthToken, owner, repo string) bool {
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "IsUserInGithubTeam", retryConfig{retry: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		catcher := grip.NewBasicCatcher()
		for _, team := range teams {
			//suppress error because it's not informative
			membership, _, err := client.Teams.GetTeamMembershipBySlug(ctx, org, team, user)
			if err != nil {
				catcher.Add(err)
				break
			}
			if membership != nil && membership.GetState() == "active" {
				return true
			}
		}

		grip.Debug(message.WrapError(catcher.Resolve(), message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get github team membership",
			"caller":  "IsUserInGithubTeam",
			"owner":   owner,
			"repo":    repo,
			"org":     org,
			"user":    user,
			"teams":   teams,
		}))

	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, oauthToken, "IsUserInGithubTeam", retryConfig{retry: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "number of teams in IsUserInGithubTeam",
		"teams":   len(teams),
	})
	for _, team := range teams {
		//suppress error because it's not informative
		membership, _, _ := client.Teams.GetTeamMembershipBySlug(ctx, org, team, user)
		if membership != nil && membership.GetState() == "active" {
			return true
		}
	}
	return false
}

// GetGithubTokenUser fetches a github user associated with an oauth token, and
// if requiredOrg is specified, checks that it belongs to that org.
// Returns user object, if it was a member of the specified org (or false if not specified),
// and error
func GetGithubTokenUser(ctx context.Context, token string, requiredOrg string) (*GithubLoginUser, bool, error) {
	ctx, httpClient, putClient := getGithubClient(ctx, fmt.Sprintf("token %s", token), "GetGithubTokenUser", retryConfig{retry: true}, nil)
	defer putClient()
	client := github.NewClient(httpClient)

	user, resp, err := client.Users.Get(ctx, "")
	if resp != nil {
		defer resp.Body.Close()
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
		isMember, _, err = client.Organizations.IsMember(ctx, requiredOrg, *user.Login)
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
	installationToken, err := getInstallationTokenWithoutOwnerRepo(ctx)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "CheckGithubAPILimit", retryConfig{retry: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		limits, _, err := client.RateLimits(ctx)
		if err == nil && limits.Core != nil && limits.Core.Remaining >= 0 {
			return int64(limits.Core.Remaining), nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to check github limit",
			"caller":  "CheckGithubAPILimit",
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "CheckGithubAPILimit", retryConfig{retry: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	limits, _, err := client.RateLimits(ctx)
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
	installationToken, err := getInstallationTokenWithoutOwnerRepo(ctx)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubUser", retryConfig{retry: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		user, _, err := client.Users.Get(ctx, loginName)
		if err == nil && user != nil && user.ID != nil && user.Login != nil {
			return user, nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":     "EVG-19966",
			"message":    "failed to github user",
			"caller":     "GetGithubUser",
			"login_name": loginName,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubUser", retryConfig{retry: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	user, _, err := client.Users.Get(ctx, loginName)
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
	installationToken, err := getInstallationTokenWithoutOwnerRepo(ctx)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GithubUserInOrganization", retryConfig{retry: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		isMember, resp, err := client.Organizations.IsMember(ctx, requiredOrganization, username)
		if err == nil {
			return isMember, nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":   "EVG-19966",
			"message":  "failed to check if user is in github organization",
			"caller":   "GithubUserInOrganization",
			"org":      requiredOrganization,
			"username": username,
			"response": resp,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GithubUserInOrganization", retryConfig{retry: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	// doesn't count against API limits
	limits, _, err := client.RateLimits(ctx)
	if err != nil {
		return false, err
	}
	if limits == nil || limits.Core == nil {
		return false, errors.New("rate limits response was empty")
	}
	if limits.Core.Remaining < 3 {
		return false, errors.New("github rate limit would be exceeded")
	}

	isMember, _, err := client.Organizations.IsMember(context.Background(), requiredOrganization, username)
	return isMember, err
}

// AppAuthorizedForOrg returns true if the given app name exists in the org's installation list,
// and has permission to write to pull requests. Returns an error if the app name exists but doesn't have permission.
func AppAuthorizedForOrg(ctx context.Context, token, requiredOrganization, name string) (bool, error) {
	installationToken, err := getInstallationTokenWithoutOwnerRepo(ctx)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "AppAuthorizedForOrg", retryConfig{retry: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		isMember, _, err := client.Organizations.IsMember(ctx, requiredOrganization, name)
		if err == nil {
			return isMember, nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to check if app is authorized for org",
			"caller":  "AppAuthorizedForOrg",
			"org":     requiredOrganization,
			"app":     name,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "AppAuthorizedForOrg", retryConfig{retry: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	opts := &github.ListOptions{PerPage: 100}
	for {
		installations, resp, err := client.Organizations.ListInstallations(ctx, requiredOrganization, opts)
		if err != nil {
			return false, err
		}
		if resp != nil {
			defer resp.Body.Close()
		}

		for _, installation := range installations.Installations {
			if installation.GetAppSlug() == name {
				prPermission := installation.GetPermissions().GetPullRequests()
				if prPermission == githubWrite {
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

func GitHubUserPermissionLevel(ctx context.Context, token, owner, repo, username string) (string, error) {
	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GithubUserPermissionLevel", retryConfig{retry404: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		permissionLevel, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, username)
		if err == nil && permissionLevel != nil && permissionLevel.Permission != nil {
			return permissionLevel.GetPermission(), nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get permissions from GitHub",
			"caller":  "GithubUserPermissionLevel",
			"owner":   owner,
			"repo":    repo,
			"user":    username,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GithubUserPermissionLevel", retryConfig{retry404: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	permissionLevel, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, username)
	if err != nil {
		return "", errors.Wrap(err, "can't get permissions from GitHub")
	}

	if permissionLevel == nil || permissionLevel.Permission == nil {
		return "", errors.Errorf("GitHub returned an invalid response to request for user permissions for '%s'", username)
	}

	return permissionLevel.GetPermission(), nil
}

// GetPullRequestMergeBase returns the merge base hash for the given PR.
// This function will retry up to 5 times, regardless of error response (unless
// error is the result of hitting an api limit)
func GetPullRequestMergeBase(ctx context.Context, token string, data GithubPatch) (string, error) {
	var commits []*github.RepositoryCommit
	installationToken, err := GetInstallationToken(ctx, data.BaseOwner, data.BaseRepo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetPullRequestMergeBase", retryConfig{retry404: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		commits, _, err := client.PullRequests.ListCommits(ctx, data.BaseOwner, data.BaseRepo, data.PRNumber, nil)

		if err == nil && len(commits) != 0 && commits[0].GetSHA() != "" {
			commit, resp, err := client.Repositories.GetCommit(ctx, data.BaseOwner, data.BaseRepo, commits[0].GetSHA(), nil)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err == nil && commit != nil && len(commit.Parents) != 0 && commit.Parents[0].GetSHA() != "" {
				return commit.Parents[0].GetSHA(), nil
			}
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":    "EVG-19966",
			"message":   "failed to get commits from GitHub",
			"caller":    "GetPullRequestMergeBase",
			"owner":     data.BaseOwner,
			"repo":      data.BaseRepo,
			"pr_number": data.PRNumber,
		}))
	}
	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetPullRequestMergeBase", retryConfig{retry404: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	commits, _, err = client.PullRequests.ListCommits(ctx, data.BaseOwner, data.BaseRepo, data.PRNumber, nil)
	if err != nil {
		return "", err
	}

	if len(commits) == 0 {
		return "", errors.New("No commits received from github")
	}
	if commits[0].GetSHA() == "" {
		return "", errors.New("hash is missing from pull request commit list")
	}

	commit, _, err := client.Repositories.GetCommit(ctx, data.BaseOwner, data.BaseRepo, commits[0].GetSHA(), nil)
	if err != nil {
		return "", err
	}

	if commit == nil {
		return "", errors.New("couldn't find commit")
	}
	if len(commit.Parents) == 0 {
		return "", errors.New("can't find pull request branch point")
	}
	if commit.Parents[0].GetSHA() == "" {
		return "", errors.New("parent hash is missing")
	}

	return commit.Parents[0].GetSHA(), nil
}

func GetGithubPullRequest(ctx context.Context, token, baseOwner, baseRepo string, prNumber int) (*github.PullRequest, error) {
	installationToken, err := GetInstallationToken(ctx, baseOwner, baseRepo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubPullRequest", retryConfig{retry404: true}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, baseOwner),
			attribute.String(githubRepoAttribute, baseRepo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		pr, _, err := client.PullRequests.Get(ctx, baseOwner, baseRepo, prNumber)
		if err == nil {
			return pr, nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":    "EVG-19966",
			"message":   "failed to get pull request from GitHub",
			"caller":    "GetGithubPullRequest",
			"owner":     baseOwner,
			"repo":      baseRepo,
			"pr_number": prNumber,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubPullRequest", retryConfig{retry404: true}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, baseOwner),
		attribute.String(githubRepoAttribute, baseRepo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	pr, _, err := client.PullRequests.Get(ctx, baseOwner, baseRepo, prNumber)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// GetGithubPullRequestDiff downloads a diff from a Github Pull Request diff
func GetGithubPullRequestDiff(ctx context.Context, token string, gh GithubPatch) (string, []Summary, error) {
	installationToken, err := GetInstallationToken(ctx, gh.BaseOwner, gh.BaseRepo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "GetGithubPullRequestDiff", retryConfig{retry404: true}, nil)
		defer putClient()
		client := github.NewClient(httpClient)

		diff, _, err := client.PullRequests.GetRaw(ctx, gh.BaseOwner, gh.BaseRepo, gh.PRNumber, github.RawOptions{Type: github.Diff})
		if err == nil {
			summaries, err := GetPatchSummaries(diff)
			if err == nil {
				return diff, summaries, nil
			}
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":    "EVG-19966",
			"message":   "failed to get pull request diff from GitHub",
			"caller":    "GetGithubPullRequestDiff",
			"owner":     gh.BaseOwner,
			"repo":      gh.BaseRepo,
			"pr_number": gh.PRNumber,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "GetGithubPullRequestDiff", retryConfig{retry404: true}, nil)
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	diff, _, err := client.PullRequests.GetRaw(ctx, gh.BaseOwner, gh.BaseRepo, gh.PRNumber, github.RawOptions{Type: github.Diff})
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

func SendCommitQueueGithubStatus(env evergreen.Environment, pr *github.PullRequest, state message.GithubState, description, versionID string) error {
	sender, err := env.GetSender(evergreen.SenderGithubStatus)
	if err != nil {
		return errors.Wrap(err, "can't get GitHub status sender")
	}

	var url string
	if versionID != "" {
		uiConfig := evergreen.UIConfig{}
		if err := uiConfig.Get(env); err == nil {
			url = fmt.Sprintf("%s/version/%s?redirect_spruce_users=true", uiConfig.Url, versionID)
		}
	}

	msg := message.GithubStatus{
		Owner:       *pr.Base.Repo.Owner.Login,
		Repo:        *pr.Base.Repo.Name,
		Ref:         *pr.Head.SHA,
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

// CreateGithubHook creates a new GitHub webhook for a repo.
func CreateGithubHook(ctx context.Context, settings evergreen.Settings, owner, repo string) (*github.Hook, error) {
	installationToken, _ := settings.CreateInstallationToken(ctx, owner, repo, nil)

	if settings.Api.GithubWebhookSecret == "" {
		return nil, errors.New("Evergreen is not configured for GitHub Webhooks")
	}

	hookObj := github.Hook{
		Active: github.Bool(true),
		Events: []string{"*"},
		Config: map[string]interface{}{
			"url":          github.String(fmt.Sprintf(githubHookURL, settings.ApiUrl)),
			"content_type": github.String("json"),
			"secret":       github.String(settings.Api.GithubWebhookSecret),
			"insecure_ssl": github.String("0"),
		},
	}

	if installationToken != "" {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "CreateGithubHook", retryConfig{}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		respHook, resp, err := client.Repositories.CreateHook(ctx, owner, repo, &hookObj)
		if resp != nil {
			defer resp.Body.Close()
			if err == nil && resp.StatusCode == http.StatusCreated && respHook != nil && respHook.ID != nil {
				return respHook, nil
			}
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to create hook",
			"caller":  "CreateGithubHook",
			"owner":   owner,
			"repo":    repo,
			"error":   err.Error(),
			"hook":    hookObj,
		}))
	}

	// Fallback to not using the GitHub app on error.
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting github oauth token")
	}

	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "CreateGithubHook", retryConfig{}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	respHook, resp, err := client.Repositories.CreateHook(ctx, owner, repo, &hookObj)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || respHook == nil || respHook.ID == nil {
		return nil, errors.New("unexpected data from GitHub")
	}
	return respHook, nil
}

// GetExistingGithubHook gets information from GitHub about an existing webhook
// for a repo.
func GetExistingGithubHook(ctx context.Context, settings evergreen.Settings, owner, repo string) (*github.Hook, error) {
	installationToken, _ := settings.CreateInstallationToken(ctx, owner, repo, nil)
	if installationToken != "" {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "ListGithubHooks", retryConfig{}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		respHooks, _, err := client.Repositories.ListHooks(ctx, owner, repo, nil)
		if err == nil {
			url := fmt.Sprintf(githubHookURL, settings.ApiUrl)
			for _, hook := range respHooks {
				if hook.Config["url"] == url {
					return hook, nil
				}
			}
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":  "EVG-19966",
			"message": "failed to get existing hook",
			"caller":  "GetExistingGithubHook",
			"owner":   owner,
			"repo":    repo,
		}))
	}

	// Fallback to not using the GitHub app on error.
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting github oauth token")
	}

	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "ListGithubHooks", retryConfig{}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	respHooks, _, err := client.Repositories.ListHooks(ctx, owner, repo, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "getting hooks for owner '%s', repo '%s'", owner, repo)
	}

	url := fmt.Sprintf(githubHookURL, settings.ApiUrl)
	for _, hook := range respHooks {
		if hook.Config["url"] == url {
			return hook, nil
		}
	}

	return nil, errors.Errorf("no matching hooks found")
}

// MergePullRequest attempts to merge the given pull request. If commits are merged one after another, Github may
// not have updated that this can be merged, so we allow retries.
func MergePullRequest(ctx context.Context, token, appToken, owner, repo, commitMessage string, prNum int, mergeOpts *github.PullRequestOptions) error {
	if appToken != "" {
		ctx, httpClient, putClient := getGithubClient(ctx, appToken, "MergePullRequest", retryConfig{}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		res, _, err := client.PullRequests.Merge(ctx, owner, repo, prNum, commitMessage, mergeOpts)
		if err == nil && res.GetMerged() {
			return nil
		}

		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":         "EVG-19966",
			"message":        "failed to merge pull request",
			"caller":         "MergePullRequest",
			"owner":          owner,
			"repo":           repo,
			"pr_number":      prNum,
			"commit_message": commitMessage,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "MergePullRequest", retryConfig{}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	res, _, err := client.PullRequests.Merge(ctx, owner, repo,
		prNum, commitMessage, mergeOpts)
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
	githubComment := &github.IssueComment{
		Body: &comment,
	}

	installationToken, err := GetInstallationToken(ctx, owner, repo, nil)
	if err != nil {
		ctx, httpClient, putClient := getGithubClient(ctx, installationToken, "PostCommentToPullRequest", retryConfig{}, []attribute.KeyValue{
			attribute.String(githubOwnerAttribute, owner),
			attribute.String(githubRepoAttribute, repo),
		})
		defer putClient()
		client := github.NewClient(httpClient)

		respComment, resp, err := client.Issues.CreateComment(ctx, owner, repo, prNum, githubComment)
		if resp != nil {
			defer resp.Body.Close()
			if err == nil && resp.StatusCode == http.StatusCreated && respComment != nil && respComment.ID != nil {
				return nil
			}
		}
		grip.Debug(message.WrapError(err, message.Fields{
			"ticket":         "EVG-19966",
			"message":        "failed to create comment",
			"caller":         "PostCommentToPullRequest",
			"owner":          owner,
			"repo":           repo,
			"pr_number":      prNum,
			"github_comment": githubComment,
		}))
	}

	// Fallback to not using the GitHub app on error.
	ctx, legacyClient, putLegacyClient := getGithubClient(ctx, token, "PostCommentToPullRequest", retryConfig{}, []attribute.KeyValue{
		attribute.String(githubOwnerAttribute, owner),
		attribute.String(githubRepoAttribute, repo),
	})
	defer putLegacyClient()
	client := github.NewClient(legacyClient)

	respComment, newResp, err := client.Issues.CreateComment(ctx, owner, repo, prNum, githubComment)
	if err != nil {
		return errors.Wrap(err, "can't access GitHub merge API")
	}

	defer newResp.Body.Close()
	if newResp.StatusCode != http.StatusCreated || respComment == nil || respComment.ID == nil {
		return errors.New("unexpected data from GitHub")
	}
	return nil
}
