package thirdparty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

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
)

func githubRetry(resp *github.Response, err error) (bool, error) {
	if _, ok := err.(*github.RateLimitError); ok {
		return false, err
	}
	if resp.StatusCode == http.StatusBadRequest {
		return false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		err = errors.Errorf("Calling github GET on %v failed: got 'unauthorized' response", resp.Request.URL.String())
		grip.Error(err)
		return false, err
	}
	if err != nil {
		grip.Errorf("failed trying to call github GET on %s: %+v", resp.Request.URL.String(), err)
		return true, err
	}
	if resp.StatusCode != http.StatusOK {
		err = errors.Errorf("Calling github GET %s got a bad response code: %v", resp.Request.URL.String(), resp.StatusCode)
	}

	rateMessage, _ := getGithubRateLimit(resp.Header)
	grip.Debugf("Github API response: %s. %s", resp.Status, rateMessage)

	return false, err
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(ctx context.Context, oauthToken, owner, repo, ref string, commitPage int) ([]*github.RepositoryCommit, int, error) {
	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return nil, 0, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)

	client := github.NewClient(httpClient)
	var commits []*github.RepositoryCommit
	var resp *github.Response
	_, _ = util.Retry(
		func() (bool, error) {
			commits, resp, err = client.Repositories.ListCommits(ctx,
				owner, repo, &github.CommitsListOptions{
					SHA: ref,
					ListOptions: github.ListOptions{
						Page: commitPage,
					},
				})

			return githubRetry(resp, err)
		}, NumGithubRetries, GithubSleepTimeSecs)

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
	defer resp.Body.Close()

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

func GetGithubAPIStatus(ctx context.Context) (string, error) {
	resp, err := githubRequest(ctx, http.MethodGet, fmt.Sprintf("%v/api/status.json", GithubStatusBase), "", nil)
	if err != nil {
		return "", errors.Wrap(err, "github request failed")
	}
	defer resp.Body.Close()

	gitStatus := struct {
		Status      string    `json:"status"`
		LastUpdated time.Time `json:"last_updated"`
	}{}

	if err = util.ReadJSONInto(resp.Body, &gitStatus); err != nil {
		return "", errors.Wrap(err, "json read failed")
	}

	return gitStatus.Status, nil
}

// GetGithubFile returns a struct that contains the contents of files within
// a repository as Base64 encoded content.
func GetGithubFile(ctx context.Context, oauthToken, owner, repo, path, hash string) (*github.RepositoryContent, error) {
	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	var opt *github.RepositoryContentGetOptions
	if len(hash) != 0 {
		opt = &github.RepositoryContentGetOptions{
			Ref: hash,
		}
	}

	var file *github.RepositoryContent
	var resp *github.Response
	_, _ = util.Retry(
		func() (bool, error) {
			file, _, resp, err = client.Repositories.GetContents(ctx, owner, repo, path, opt)
			return githubRetry(resp, err)
		}, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil, FileNotFoundError{filepath: path}
	}
	if err != nil {
		errMsg := fmt.Sprintf("error querying '%s/%s' for '%s': %v", owner, repo, path, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from github for '%s/%s' for '%s'", owner, repo, path)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	defer resp.Body.Close()

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
	ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancel()

	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return "", errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	var compare *github.CommitsComparison
	var resp *github.Response
	_, _ = util.Retry(
		func() (bool, error) {
			compare, resp, err = client.Repositories.CompareCommits(ctx,
				repoOwner, repo, baseRevision, currentCommitHash)
			return githubRetry(resp, err)
		}, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if err != nil {
		errMsg := fmt.Sprintf("error getting merge base commit response for '%s/%s'@%s..%s: %v", repoOwner, repo, baseRevision, currentCommitHash, err)
		grip.Error(errMsg)
		return "", APIResponseError{errMsg}
	}
	defer resp.Body.Close()

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
	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    repoOwner + "/" + repo,
	})

	var commit *github.RepositoryCommit
	var resp *github.Response
	_, _ = util.Retry(
		func() (bool, error) {
			commit, resp, err = client.Repositories.GetCommit(ctx, repoOwner, repo, githash)
			return githubRetry(resp, err)
		}, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if err != nil {
		err = errors.Wrapf(err, "problem querying repo %s/%s for %s", repoOwner, repo, githash)
		grip.Error(message.WrapError(errors.Cause(err), message.Fields{
			"commit":  githash,
			"repo":    repoOwner + "/" + repo,
			"message": "problem querying repo",
		}))
		return nil, APIResponseError{err.Error()}
	}
	defer resp.Body.Close()

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
	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	grip.Debugf("requesting github commit for '%s/%s': branch: %s\n", repoOwner, repo, branch)

	var branchEvent *github.Branch
	var resp *github.Response
	_, _ = util.Retry(
		func() (bool, error) {
			branchEvent, resp, err = client.Repositories.GetBranch(ctx, repoOwner, repo, branch)
			return githubRetry(resp, err)
		}, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if err != nil {
		errMsg := fmt.Sprintf("error querying  '%s/%s': branch: '%s': %v", repoOwner, repo, branch, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	defer resp.Body.Close()

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
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	return client.Do(req)
}

// tryGithubPost posts the data to the Github api endpoint with the url given
func tryGithubPost(ctx context.Context, url string, oauthToken string, data interface{}) (resp *http.Response, err error) {
	grip.Errorf("Attempting GitHub API POST at ‘%s’", url)
	retryFail, err := util.Retry(func() (bool, error) {
		resp, err = githubRequest(ctx, "POST", url, oauthToken, data)
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
	}, NumGithubRetries, GithubSleepTimeSecs*time.Second)

	if err != nil {
		// couldn't post it
		if retryFail {
			grip.Errorf("Github POST to '%s' used up all retries.", url)
		}
		return nil, errors.WithStack(err)
	}

	return
}

// getGithubRateLimit interprets the limit headers, and produces an increasingly
// alarmed message (for the caller to log) as we get closer and closer
func getGithubRateLimit(header http.Header) (message string, loglevel level.Priority) {
	h := (map[string][]string)(header)
	limStr, okLim := h["X-Ratelimit-Limit"]
	remStr, okRem := h["X-Ratelimit-Remaining"]

	// ensure that we were able to read the rate limit header
	if !okLim || !okRem || len(limStr) == 0 || len(remStr) == 0 {
		loglevel = level.Warning
		message = "Could not get rate limit data"
		return
	}

	// parse the rate limits
	lim, limErr := strconv.ParseInt(limStr[0], 10, 0) // parse in decimal to int
	rem, remErr := strconv.ParseInt(remStr[0], 10, 0)

	// ensure we successfully parsed the rate limits
	if limErr != nil || remErr != nil {
		loglevel = level.Warning
		message = fmt.Sprintf("Could not parse rate limit data: limit=%q, rate=%t",
			limStr, okLim)
		return
	}

	// We're in good shape
	if rem > int64(0.1*float32(lim)) {
		loglevel = level.Info
		message = fmt.Sprintf("Rate limit: %v/%v", rem, lim)
		return
	}

	// we're running short
	if rem > 20 {
		loglevel = level.Warning
		message = fmt.Sprintf("Rate limit significantly low: %v/%v", rem, lim)
		return
	}

	// we're in trouble
	loglevel = level.Error
	message = fmt.Sprintf("Throttling required - rate limit almost exhausted: %v/%v", rem, lim)
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

// GetGithubUser fetches a github user associated with an oauth token, and
// if requiredOrg is specified, checks that it belongs to that org.
// Returns user object, if it was a member of the specified org (or false if not specified),
// and error
func GetGithubUser(ctx context.Context, token string, requiredOrg string) (*GithubLoginUser, bool, error) {
	httpClient, err := util.GetHttpClientForOauth2(fmt.Sprintf("token %s", token))
	if err != nil {
		return nil, false, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	user, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	defer resp.Body.Close()
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
		isMember, _, err = client.Organizations.IsMember(context.TODO(), requiredOrg, *user.Login)
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
		Id:               *user.ID,
		Company:          *user.Company,
		EmailAddress:     *user.Email,
		OrganizationsURL: *user.OrganizationsURL,
	}, isMember, err
}

// CheckGithubAPILimit queries Github for the number of API requests remaining
func CheckGithubAPILimit(ctx context.Context, oauthToken string) (int64, error) {
	httpClient, err := util.GetHttpClientForOauth2(oauthToken)
	if err != nil {
		return 0, errors.Wrap(err, "can't fetch data from github")
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	limits, resp, err := client.RateLimits(ctx)
	if err != nil {
		grip.Errorf("github GET rate limit failed: %+v", err)
		return 0, err
	}
	defer resp.Body.Close()

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
