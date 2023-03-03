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
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	NumGithubAttempts   = 3
	GithubRetryMinDelay = time.Second
	GithubAccessURL     = "https://github.com/login/oauth/access_token"
	githubHookURL       = "%s/rest/v2/hooks/github"

	Github502Error   = "502 Server Error"
	commitObjectType = "commit"
	tagObjectType    = "tag"

	GithubInvestigation = "Github API Limit Investigation"
)

var UnblockedGithubStatuses = []string{
	githubPrBehind,
	githubPrClean,
	githubPrDirty,
	githubPrDraft,
	githubPrHas_Hooks,
	githubPrUnknown,
	githubPrUnstable,
}

const (
	// All PR statuses except for "blocked" based on statuses listed here:
	// https://docs.github.com/en/graphql/reference/enums#mergestatestatus
	// Because the pr.MergeableState is not documented, it can change without
	// notice. That's why we want to only allow fields we know to be unblocked
	// rather than simply blocking the "blocked" status. That way if it does
	// change, it doesn't fail silently.
	githubPrBehind    = "behind"
	githubPrClean     = "clean"
	githubPrDirty     = "dirty"
	githubPrDraft     = "draft"
	githubPrHas_Hooks = "has_hooks"
	githubPrUnknown   = "unknown"
	githubPrUnstable  = "unstable"
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

// SendGithubStatusInput is the input to the SendVersionStatusToGithub function and contains
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
	GithubPatchPRNumberKey  = bsonutil.MustHaveTag(GithubPatch{}, "PRNumber")
	GithubPatchBaseOwnerKey = bsonutil.MustHaveTag(GithubPatch{}, "BaseOwner")
	GithubPatchBaseRepoKey  = bsonutil.MustHaveTag(GithubPatch{}, "BaseRepo")
)

func githubShouldRetry(caller string) utility.HTTPRetryFunction {
	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		if index >= NumGithubAttempts {
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

		return false
	}
}

// githubShouldRetryWith404s allows HTTP requests to respond event when 404s
// are returned.
func githubShouldRetryWith404s(caller string) utility.HTTPRetryFunction {
	return func(index int, req *http.Request, resp *http.Response, err error) bool {
		if index >= NumGithubAttempts {
			return false
		}

		if resp == nil {
			grip.Info(message.Fields{
				"ticket":    GithubInvestigation,
				"message":   "resp is nil in githubShouldRetryWith404s",
				"caller":    caller,
				"retry_num": index,
			})
			return true
		}

		limit := parseGithubRateLimit(resp.Header)
		if limit.Remaining == 0 {
			return false
		}

		if resp.StatusCode == http.StatusNotFound {
			logGitHubRateLimit(limit)
			grip.Info(message.Fields{
				"ticket":    GithubInvestigation,
				"message":   fmt.Sprintf("hit %d in githubShouldRetryWith404s", http.StatusNotFound),
				"caller":    caller,
				"retry_num": index,
			})
			return true
		}

		retryFn := githubShouldRetry(caller)
		return retryFn(index, req, resp, err)
	}
}

func getGithubClientRetryWith404s(token, caller string) *http.Client {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClientRetryWith404s",
		"caller":  caller,
	})
	return utility.GetOauth2CustomHTTPRetryableClient(
		token,
		githubShouldRetryWith404s(caller),
		utility.RetryHTTPDelay(utility.RetryOptions{
			MaxAttempts: NumGithubAttempts,
			MinDelay:    GithubRetryMinDelay,
		}),
	)
}

func getGithubClient(token, caller string) *http.Client {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClient",
		"caller":  caller,
	})

	return utility.GetOAuth2HTTPClient(token)
}

func getGithubClientRetry(token, caller string) *http.Client {
	grip.Info(message.Fields{
		"ticket":  GithubInvestigation,
		"message": "called getGithubClientRetry",
		"caller":  caller,
	})
	return utility.GetOauth2CustomHTTPRetryableClient(
		token,
		githubShouldRetry(caller),
		utility.RetryHTTPDelay(utility.RetryOptions{
			MaxAttempts: NumGithubAttempts,
			MinDelay:    GithubRetryMinDelay,
		}),
	)
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, oauthToken, owner, repo, ref string, until time.Time, commitPage int) ([]*github.RepositoryCommit, int, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetGithubCommits")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	options := github.CommitsListOptions{
		SHA: ref,
		ListOptions: github.ListOptions{
			Page: commitPage,
		},
	}
	if !utility.IsZeroTime(until) {
		options.Until = until
	}

	commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, &options)
	if resp != nil {
		defer resp.Body.Close()
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
func GetGithubFile(ctx context.Context, oauthToken, owner, repo, path, ref string) (*github.RepositoryContent, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetGithubFile")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	var opt *github.RepositoryContentGetOptions
	if len(ref) != 0 {
		opt = &github.RepositoryContentGetOptions{
			Ref: ref,
		}
	}

	file, _, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, opt)
	if resp != nil {
		defer resp.Body.Close()
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

// SendVersionStatusToGithub sends a pending status to a Github PR patch
// associated with a given version.
func SendVersionStatusToGithub(input SendGithubStatusInput) error {
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
	uiConfig := evergreen.UIConfig{}
	if err = uiConfig.Get(env); err != nil {
		return errors.Wrap(err, "retrieving UI config")
	}
	urlBase := uiConfig.Url
	if urlBase == "" {
		return errors.New("url base doesn't exist")
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
func GetGithubMergeBaseRevision(ctx context.Context, oauthToken, repoOwner, repo, baseRevision, currentCommitHash string) (string, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetGithubMergeBaseRevision")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	compare, resp, err := client.Repositories.CompareCommits(ctx,
		repoOwner, repo, baseRevision, currentCommitHash)
	if resp != nil {
		defer resp.Body.Close()
		if err != nil {
			return "", parseGithubErrorResponse(resp)
		}
	} else {
		apiErr := errors.Errorf("nil response from merge base commit response for '%s/%s'@%s..%s: %v", repoOwner, repo, baseRevision, currentCommitHash, err)
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

func GetCommitEvent(ctx context.Context, oauthToken, repoOwner, repo, githash string) (*github.RepositoryCommit, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetCommitEvent")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    repoOwner + "/" + repo,
	})

	commit, resp, err := client.Repositories.GetCommit(ctx, repoOwner, repo, githash)
	if resp != nil {
		defer resp.Body.Close()
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		err = errors.Wrapf(err, "nil response from repo %s/%s for %s", repoOwner, repo, githash)
		grip.Error(message.WrapError(errors.Cause(err), message.Fields{
			"commit":  githash,
			"repo":    repoOwner + "/" + repo,
			"message": "problem querying repo",
		}))
		return nil, APIResponseError{err.Error()}
	}

	msg := message.Fields{
		"operation": "github api query",
		"size":      resp.ContentLength,
		"status":    resp.Status,
		"query":     githash,
		"repo":      repoOwner + "/" + repo,
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
func GetCommitDiff(ctx context.Context, oauthToken, repoOwner, repo, sha string) (string, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetCommitDiff")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	commit, resp, err := client.Repositories.GetCommitRaw(ctx, repoOwner, repo, sha, github.RawOptions{Type: github.Diff})
	if resp != nil {
		defer resp.Body.Close()
		if err != nil {
			return "", parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from '%s/%s': sha: '%s': %v", repoOwner, repo, sha, err)
		grip.Error(message.Fields{
			"message": errMsg,
			"owner":   repoOwner,
			"repo":    repo,
			"sha":     sha,
		})
		return "", APIResponseError{errMsg}
	}

	return commit, nil
}

// GetBranchEvent gets the head of the a given branch via an API call to GitHub
func GetBranchEvent(ctx context.Context, oauthToken, repoOwner, repo, branch string) (*github.Branch, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetBranchEvent")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", repoOwner, repo, branch)

	branchEvent, resp, err := client.Repositories.GetBranch(ctx, repoOwner, repo, branch)
	if resp != nil {
		defer resp.Body.Close()
		if err != nil {
			return nil, parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from github for '%s/%s': branch: '%s': %v", repoOwner, repo, branch, err)
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
		MaxAttempts: NumGithubAttempts,
		MinDelay:    GithubRetryMinDelay,
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
	resp, err := tryGithubPost(ctx, GithubAccessURL, "", authParameters)
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
func GetTaggedCommitFromGithub(ctx context.Context, oauthToken, owner, repo, tag string) (string, error) {
	client := getGithubClientRetry(oauthToken, "GetTaggedCommitFromGithub")
	defer utility.PutHTTPClient(client)

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs/tags/%s", owner, repo, tag)
	resp, err := client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", errors.Wrap(err, "failed to get tag information from Github")
	}
	if resp == nil {
		return "", errors.New("invalid github response")
	}

	respBody, err := io.ReadAll(resp.Body)
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
		githubClient := github.NewClient(client)
		annotatedTagResp, _, err = githubClient.Git.GetTag(ctx, owner, repo, tagSha)
		if err != nil {
			return "", errors.Wrapf(err, "error getting tag '%s' with SHA '%s'", tag, tagSha)
		}
		sha = annotatedTagResp.GetObject().GetSHA()
	default:
		return "", errors.Errorf("unrecognized object type '%s'", tagResp.GetObject().GetType())
	}

	if tagSha == "" {
		return "", errors.New("empty SHA from Github")
	}

	return sha, nil
}

func IsUserInGithubTeam(ctx context.Context, teams []string, org, user, oauthToken string) bool {
	httpClient := getGithubClientRetry(oauthToken, "IsUserInGithubTeam")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

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
	httpClient := getGithubClientRetry(fmt.Sprintf("token %s", token), "GetGithubTokenUser")
	defer utility.PutHTTPClient(httpClient)
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
		return nil, false, errors.New("Github user is missing required data")
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
func CheckGithubAPILimit(ctx context.Context, oauthToken string) (int64, error) {
	httpClient := getGithubClientRetry(oauthToken, "CheckGithubAPILimit")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	limits, resp, err := client.RateLimits(ctx)
	if resp != nil {
		defer resp.Body.Close()
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
func GetGithubUser(ctx context.Context, oauthToken, loginName string) (*github.User, error) {
	httpClient := getGithubClientRetry(oauthToken, "GetGithubUser")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

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
	httpClient := getGithubClientRetry(token, "GithubUserInOrganization")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

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

func GitHubUserPermissionLevel(ctx context.Context, token, owner, repo, username string) (string, error) {
	httpClient := getGithubClientRetryWith404s(token, "GithubUserPermissionLevel")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	permissionLevel, _, err := client.Repositories.GetPermissionLevel(ctx, owner, repo, username)
	if err != nil {
		return "", errors.Wrap(err, "can't get permissions from GitHub")
	}
	if permissionLevel == nil || permissionLevel.Permission == nil {
		return "", errors.Errorf("GitHub returned an invalid response to request for user permissions for '%s'", username)
	}

	return *permissionLevel.Permission, nil
}

// GetPullRequestMergeBase returns the merge base hash for the given PR.
// This function will retry up to 5 times, regardless of error response (unless
// error is the result of hitting an api limit)
func GetPullRequestMergeBase(ctx context.Context, token string, data GithubPatch) (string, error) {
	httpClient := getGithubClientRetryWith404s(token, "GetPullRequestMergeBase")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	commits, _, err := client.PullRequests.ListCommits(ctx, data.BaseOwner, data.BaseRepo, data.PRNumber, nil)
	if err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", errors.New("No commits received from github")
	}
	if commits[0].SHA == nil {
		return "", errors.New("hash is missing from pull request commit list")
	}

	commit, _, err := client.Repositories.GetCommit(ctx, data.BaseOwner, data.BaseRepo, *commits[0].SHA)
	if err != nil {
		return "", err
	}
	if commit == nil {
		return "", errors.New("couldn't find commit")
	}
	if len(commit.Parents) == 0 {
		return "", errors.New("can't find pull request branch point")
	}
	if commit.Parents[0].SHA == nil {
		return "", errors.New("parent hash is missing")
	}

	return *commit.Parents[0].SHA, nil
}

func GetGithubPullRequest(ctx context.Context, token, baseOwner, baseRepo string, prNumber int) (*github.PullRequest, error) {
	httpClient := getGithubClientRetryWith404s(token, "GetGithubPullRequest")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	pr, _, err := client.PullRequests.Get(ctx, baseOwner, baseRepo, prNumber)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

func GetGithubPullRequestCommits(ctx context.Context, token, owner, repo string, prNumber int) ([]*github.RepositoryCommit, error) {
	httpClient := getGithubClientRetryWith404s(token, "GetGithubPullRequestCommits")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	commits, _, err := client.PullRequests.ListCommits(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, errors.New("No commits received from github")
	}

	return commits, nil
}

// GetGithubPullRequestReviews retrieves a list of reviews for the given PR.
func GetGithubPullRequestReviews(ctx context.Context, token, owner, repo string, prNumber int, reviewPage int) ([]*github.PullRequestReview, int, error) {
	httpClient := getGithubClientRetryWith404s(token, "GetGithubPullRequestCommits")
	defer utility.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	opts := &github.ListOptions{
		PerPage: 100,
		Page:    reviewPage,
	}

	reviews, resp, err := client.PullRequests.ListReviews(ctx, owner, repo, prNumber, opts)
	if resp != nil {
		defer resp.Body.Close()
		if err != nil {
			return nil, 0, parseGithubErrorResponse(resp)
		}
	} else {
		errMsg := fmt.Sprintf("nil response from query for PR reviews in '%s/%s' prNumber %d : %v", owner, repo, prNumber, err)
		return nil, 0, APIResponseError{errMsg}
	}

	return reviews, resp.NextPage, nil
}

// GetGithubPullRequestDiff downloads a diff from a Github Pull Request diff
func GetGithubPullRequestDiff(ctx context.Context, token string, gh GithubPatch) (string, []Summary, error) {
	httpClient := getGithubClientRetryWith404s(token, "GetGithubPullRequestDiff")

	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

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
func CreateGithubHook(ctx context.Context, settings evergreen.Settings, owner string, repo string) (*github.Hook, error) {
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, err
	}
	if settings.Api.GithubWebhookSecret == "" {
		return nil, errors.New("Evergreen is not configured for GitHub Webhooks")
	}

	httpClient := getGithubClient(token, "CreateGithubHook")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
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
	token, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, errors.Wrap(err, "getting GitHub token")
	}

	httpClient := getGithubClient(token, "ListGithubHooks")
	defer utility.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

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

func MergePullRequest(ctx context.Context, token, owner, repo, commitMessage string, prNum int, mergeOpts *github.PullRequestOptions) error {
	httpClient := getGithubClient(token, "MergePullRequest")
	defer utility.PutHTTPClient(httpClient)
	githubClient := github.NewClient(httpClient)
	res, _, err := githubClient.PullRequests.Merge(ctx, owner, repo,
		prNum, commitMessage, mergeOpts)
	if err != nil {
		return errors.Wrap(err, "can't access GitHub merge API")
	}
	if !res.GetMerged() {
		return errors.Errorf("Github refused to merge PR '%s/%s:%d': '%s'", owner, repo, prNum, res.GetMessage())
	}
	return nil
}
