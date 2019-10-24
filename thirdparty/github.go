package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	NumGithubRetries    = 5
	GithubSleepTimeSecs = 1 * time.Second
	GithubStatusBase    = "https://status.github.com"
	GithubAccessURL     = "https://github.com/login/oauth/access_token"

	GithubAPIStatusMinor = "minor"
	GithubAPIStatusMajor = "major"
	GithubAPIStatusGood  = "good"

	Github502Error = "502 Server Error"

	githubAcceptDiff = "application/vnd.github.v3.diff"
)

func githubShouldRetry(attempt rehttp.Attempt) bool {
	url := attempt.Request.URL.String()

	if attempt.Error != nil {
		grip.Errorf("failed trying to call github %s on %s: %+v", attempt.Request.Method, url, attempt.Error)
		return rehttp.RetryTemporaryErr()(attempt)
	}

	if attempt.Response == nil {
		return true
	}

	if attempt.Response.StatusCode >= http.StatusBadRequest {
		grip.Error(errors.Errorf("Calling github %s on %s got a bad response code: %v", attempt.Request.Method, url, attempt.Response.StatusCode))
	}

	limit := parseGithubRateLimit(attempt.Response.Header)
	if limit.Remaining == 0 {
		return false
	}

	if attempt.Response.StatusCode == http.StatusBadGateway {
		return true
	}

	rateMessage, _ := getGithubRateLimit(attempt.Response.Header)
	grip.Debugf("Github API response: %s. %s", attempt.Response.Status, rateMessage)

	return false
}

// githubShouldRetryWith404s allows HTTP requests to respond event when 404s
// are returned.
func githubShouldRetryWith404s(attempt rehttp.Attempt) bool {
	if attempt.Response == nil {
		return true
	}

	limit := parseGithubRateLimit(attempt.Response.Header)
	if limit.Remaining == 0 {
		return false
	}

	if attempt.Response.StatusCode == http.StatusNotFound {
		return true
	}

	return githubShouldRetry(attempt)
}

func getGithubClient(token string) (*http.Client, error) {
	all := rehttp.RetryAll(rehttp.RetryMaxRetries(NumGithubRetries-1), githubShouldRetry)
	return util.GetRetryableOauth2HTTPClient(token, all, util.RehttpDelay(GithubSleepTimeSecs, NumGithubRetries))
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, oauthToken, owner, repo, ref string, commitPage int) ([]*github.RepositoryCommit, int, error) {
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return nil, 0, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo,
		&github.CommitsListOptions{
			SHA: ref,
			ListOptions: github.ListOptions{
				Page: commitPage,
			},
		})
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		errMsg := fmt.Sprintf("error querying for commits in '%s/%s' ref %s : %v", owner, repo, ref, err)
		grip.Error(errMsg)
		return nil, 0, APIResponseError{errMsg}
	}
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url '%s/%s' ref %s", owner, repo, ref)
		grip.Error(errMsg)
		return nil, 0, APIResponseError{errMsg}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, 0, ResponseReadError{err.Error()}
		}
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, 0, APIRequestError{Message: string(respBody)}
		}
		return nil, 0, requestError
	}

	return commits, resp.NextPage, nil
}

// GetGithubFile returns a struct that contains the contents of files within
// a repository as Base64 encoded content.
func GetGithubFile(ctx context.Context, oauthToken, owner, repo, path, hash string) (*github.RepositoryContent, error) {
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	var opt *github.RepositoryContentGetOptions
	if len(hash) != 0 {
		opt = &github.RepositoryContentGetOptions{
			Ref: hash,
		}
	}

	file, _, resp, err := client.Repositories.GetContents(ctx, owner, repo, path, opt)
	if resp != nil {
		defer resp.Body.Close()
	}
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from github for '%s/%s' for '%s'", owner, repo, path)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, FileNotFoundError{filepath: path}
	}
	if err != nil {
		errMsg := fmt.Sprintf("error querying '%s/%s' for '%s': %v", owner, repo, path, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	if resp.StatusCode != http.StatusOK {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, ResponseReadError{err.Error()}
		}

		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
	}

	if file == nil || file.Content == nil {
		return nil, APIRequestError{Message: "file is nil"}
	}

	return file, nil
}

func GetGithubMergeBaseRevision(ctx context.Context, oauthToken, repoOwner, repo, baseRevision, currentCommitHash string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return "", errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	compare, resp, err := client.Repositories.CompareCommits(ctx,
		repoOwner, repo, baseRevision, currentCommitHash)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		errMsg := fmt.Sprintf("error getting merge base commit response for '%s/%s'@%s..%s: %v", repoOwner, repo, baseRevision, currentCommitHash, err)
		grip.Error(errMsg)
		return "", APIResponseError{errMsg}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", ResponseReadError{err.Error()}
		}
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return "", APIRequestError{Message: string(respBody)}
		}
		return "", requestError
	}

	if compare == nil || compare.MergeBaseCommit == nil || compare.MergeBaseCommit.SHA == nil {
		return "", APIRequestError{Message: "missing data from github compare response"}
	}

	return *compare.MergeBaseCommit.SHA, nil
}

func GetCommitEvent(ctx context.Context, oauthToken, repoOwner, repo, githash string) (*github.RepositoryCommit, error) {
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    repoOwner + "/" + repo,
	})

	commit, resp, err := client.Repositories.GetCommit(ctx, repoOwner, repo, githash)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		err = errors.Wrapf(err, "problem querying repo %s/%s for %s", repoOwner, repo, githash)
		grip.Error(message.WrapError(errors.Cause(err), message.Fields{
			"commit":  githash,
			"repo":    repoOwner + "/" + repo,
			"message": "problem querying repo",
		}))
		return nil, APIResponseError{err.Error()}
	}

	grip.Debug(message.Fields{
		"operation": "github api query",
		"size":      resp.ContentLength,
		"status":    resp.Status,
		"commit":    githash,
		"repo":      repoOwner + "/" + repo,
	})

	if resp.StatusCode != http.StatusOK {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, ResponseReadError{err.Error()}
		}
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
	}
	if commit == nil {
		return nil, errors.New("commit not found in github")
	}

	return commit, nil
}

// GetBranchEvent gets the head of the a given branch via an API call to GitHub
func GetBranchEvent(ctx context.Context, oauthToken, repoOwner, repo, branch string) (*github.Branch, error) {
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", repoOwner, repo, branch)

	branchEvent, resp, err := client.Repositories.GetBranch(ctx, repoOwner, repo, branch)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		errMsg := fmt.Sprintf("error querying  '%s/%s': branch: '%s': %v", repoOwner, repo, branch, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, ResponseReadError{err.Error()}
		}
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
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
		req.Body = ioutil.NopCloser(bytes.NewReader(jsonBytes))
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
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	return client.Do(req)
}

// tryGithubPost posts the data to the Github api endpoint with the url given
func tryGithubPost(ctx context.Context, url string, oauthToken string, data interface{}) (resp *http.Response, err error) {
	grip.Errorf("Attempting GitHub API POST at ‘%s’", url)
	err = util.Retry(ctx, func() (bool, error) {
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
		// read the results
		rateMessage, loglevel := getGithubRateLimit(resp.Header)

		grip.Logf(loglevel, "Github API response: %v. %v", resp.Status, rateMessage)
		return false, nil
	}, NumGithubRetries, GithubSleepTimeSecs*time.Second, 0)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return
}

func parseGithubRateLimit(h http.Header) github.Rate {
	limitStr := h.Get("X-Ratelimit-Limit")
	remStr := h.Get("X-Ratelimit-Remaining")

	lim, _ := strconv.Atoi(limitStr)
	rem, _ := strconv.Atoi(remStr)

	return github.Rate{
		Limit:     lim,
		Remaining: rem,
	}
}

// getGithubRateLimit interprets the limit headers, and produces an increasingly
// alarmed message (for the caller to log) as we get closer and closer
func getGithubRateLimit(header http.Header) (message string, loglevel level.Priority) {
	limit := parseGithubRateLimit(header)
	if limit.Limit == 0 {
		loglevel = level.Warning
		message = "Could not get rate limit data"
		return
	}

	// We're in good shape
	if limit.Remaining > int(0.1*float32(limit.Limit)) {
		loglevel = level.Info
		message = fmt.Sprintf("Rate limit: %v/%v", limit.Remaining, limit.Limit)
		return
	}

	// we're running short
	if limit.Remaining < 20 {
		loglevel = level.Warning
		message = fmt.Sprintf("Rate limit significantly low: %v/%v", limit.Remaining, limit.Limit)
		return
	}

	// we're in trouble
	loglevel = level.Error
	message = fmt.Sprintf("Throttling required - rate limit almost exhausted: %v/%v", limit.Remaining, limit.Limit)
	return
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
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}
	grip.Debugf("GitHub API response: %s. %d bytes", resp.Status, len(respBody))

	if err = json.Unmarshal(respBody, &githubResponse); err != nil {
		return nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return
}

// GetGithubTokenUser fetches a github user associated with an oauth token, and
// if requiredOrg is specified, checks that it belongs to that org.
// Returns user object, if it was a member of the specified org (or false if not specified),
// and error
func GetGithubTokenUser(ctx context.Context, token string, requiredOrg string) (*GithubLoginUser, bool, error) {
	httpClient, err := getGithubClient(fmt.Sprintf("token %s", token))
	if err != nil {
		return nil, false, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	user, resp, err := client.Users.Get(ctx, "")
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	if resp.StatusCode != http.StatusOK {
		var respBody []byte
		respBody, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, false, ResponseReadError{err.Error()}
		}
		return nil, false, APIResponseError{string(respBody)}
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
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return 0, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)

	limits, resp, err := client.RateLimits(ctx)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		grip.Errorf("github GET rate limit failed: %+v", err)
		return 0, err
	}

	rateMessage, _ := getGithubRateLimit(resp.Header)
	grip.Debugf("Github API response: %s. %s", resp.Status, rateMessage)

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
	httpClient, err := getGithubClient(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)
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
	httpClient, err := getGithubClient(token)
	if err != nil {
		return false, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)

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
	all := rehttp.RetryAll(rehttp.RetryMaxRetries(NumGithubRetries-1), githubShouldRetryWith404s)
	httpClient, err := util.GetRetryableOauth2HTTPClient(token, all, util.RehttpDelay(GithubSleepTimeSecs, NumGithubRetries))
	if err != nil {
		return "", errors.Wrap(err, "can't get http client")
	}
	defer util.PutHTTPClient(httpClient)

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
func GetPullRequestMergeBase(ctx context.Context, token string, data patch.GithubPatch) (string, error) {
	all := rehttp.RetryAll(rehttp.RetryMaxRetries(NumGithubRetries-1), githubShouldRetryWith404s)
	httpClient, err := util.GetRetryableOauth2HTTPClient(token, all, util.RehttpDelay(GithubSleepTimeSecs, NumGithubRetries))

	if err != nil {
		return "", errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)

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

func GetGithubPullRequest(ctx context.Context, token, baseOwner, baseRepo string, PRNumber int) (*github.PullRequest, error) {
	all := rehttp.RetryAll(rehttp.RetryMaxRetries(NumGithubRetries-1), githubShouldRetryWith404s)
	httpClient, err := util.GetRetryableOauth2HTTPClient(token, all, util.RehttpDelay(GithubSleepTimeSecs, NumGithubRetries))

	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHTTPClient(httpClient)

	client := github.NewClient(httpClient)

	pr, _, err := client.PullRequests.Get(ctx, baseOwner, baseRepo, PRNumber)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// GetGithubDiff downloads a diff from a Github Pull Request diff. This function
// does not use go-github because this operation is not supported
func GetGithubPullRequestDiff(ctx context.Context, token string, gh patch.GithubPatch) (string, []patch.Summary, error) {
	all := rehttp.RetryAll(rehttp.RetryMaxRetries(NumGithubRetries-1), githubShouldRetryWith404s)
	client, err := util.GetRetryableOauth2HTTPClient(token, all, util.RehttpDelay(GithubSleepTimeSecs, NumGithubRetries))
	if err != nil {
		return "", nil, errors.Wrap(err, "error getting http client")
	}
	defer util.PutHTTPClient(client)

	req, err := http.NewRequest("GET", buildPatchURL(gh), nil)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to create github request")
	}

	diff, err := doGithubRequest(client, req, githubAcceptDiff)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to fetch diff from github")
	}

	summaries, err := GetPatchSummaries(diff)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get patch summary")
	}

	return diff, summaries, nil
}

func doGithubRequest(client *http.Client, req *http.Request, accept string) (string, error) {
	req.Header.Del("Accept")
	req.Header.Add("Accept", accept)

	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch data from github")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("Expected 200 OK, got %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	if resp.ContentLength > patch.SizeLimit || resp.ContentLength == 0 {
		return "", errors.Errorf("Patch contents must be at least 1 byte and no greater than %d bytes; was %d bytes",
			patch.SizeLimit, resp.ContentLength)
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response body from github response")
	}

	return string(bytes), nil
}

// buildPatchURL creates a URL to enable downloading patch files through the
// Github API
func buildPatchURL(gp patch.GithubPatch) string {
	url := &url.URL{
		Scheme: "https",
		Host:   "api.github.com",
		Path: fmt.Sprintf("/repos/%s/%s/pulls/%d.diff", gp.BaseOwner,
			gp.BaseRepo, gp.PRNumber),
	}

	return url.String()
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
