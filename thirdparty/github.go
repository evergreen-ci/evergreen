package thirdparty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	GithubBase          = "https://github.com"
	NumGithubRetries    = 5
	GithubSleepTimeSecs = 1
	GithubAPIBase       = "https://api.github.com"
	GithubStatusBase    = "https://status.github.com"

	GithubAPIStatusMinor = "minor"
	GithubAPIStatusMajor = "major"
	GithubAPIStatusGood  = "good"
)

type GithubUser struct {
	Active       bool   `json:"active"`
	DispName     string `json:"display-name"`
	EmailAddress string `json:"email"`
	FirstName    string `json:"first-name"`
	LastName     string `json:"last-name"`
	Name         string `json:"name"`
}

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(oauthToken, commitsURL string) (
	githubCommits []GithubCommit, header http.Header, err error) {
	resp, err := tryGithubGet(oauthToken, commitsURL)
	if resp != nil {
		defer resp.Body.Close()
	}
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url '%v'", commitsURL)
		grip.Error(errMsg)
		return nil, nil, APIResponseError{errMsg}
	}
	if err != nil {
		errMsg := fmt.Sprintf("error querying '%v': %v", commitsURL, err)
		grip.Error(errMsg)
		return nil, nil, APIResponseError{errMsg}
	}

	header = resp.Header
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, ResponseReadError{err.Error()}
	}

	grip.Debugf("Github API response: %s. %d bytes", resp.Status, len(respBody))

	if resp.StatusCode != http.StatusOK {
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, nil, APIRequestError{Message: string(respBody)}
		}
		return nil, nil, requestError
	}

	if err = json.Unmarshal(respBody, &githubCommits); err != nil {
		return nil, nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return
}

func GetGithubAPIStatus() (string, error) {
	req, err := http.NewRequest(evergreen.MethodGet, fmt.Sprintf("%v/api/status.json", GithubStatusBase), nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "github request failed")
	}

	gitStatus := struct {
		Status      string    `json:"status"`
		LastUpdated time.Time `json:"last_updated"`
	}{}

	err = util.ReadJSONInto(resp.Body, &gitStatus)
	if err != nil {
		return "", errors.Wrap(err, "json read failed")
	}

	return gitStatus.Status, nil
}

// GetGithubFile returns a struct that contains the contents of files within
// a repository as Base64 encoded content.
func GetGithubFile(oauthToken, fileURL string) (githubFile *GithubFile, err error) {
	resp, err := tryGithubGet(oauthToken, fileURL)
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url '%v'", fileURL)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}
	defer resp.Body.Close()

	if err != nil {
		errMsg := fmt.Sprintf("error querying '%v': %v", fileURL, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}

	if resp.StatusCode != http.StatusOK {
		grip.Errorf("Github API response: ‘%s’", resp.Status)
		if resp.StatusCode == http.StatusNotFound {
			return nil, FileNotFoundError{fileURL}
		}
		return nil, errors.Errorf("github API returned status '%v'", resp.Status)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}

	grip.Debugf("Github API response: %s. %d bytes", resp.Status, len(respBody))

	if resp.StatusCode != http.StatusOK {
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
	}

	if err = json.Unmarshal(respBody, &githubFile); err != nil {
		return nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return
}

func GetGitHubMergeBaseRevision(oauthToken, repoOwner, repo, baseRevision string, currentCommit *GithubCommit) (string, error) {
	if currentCommit == nil {
		return "", errors.New("no recent commit found")
	}
	url := fmt.Sprintf("%v/repos/%v/%v/compare/%v:%v...%v:%v",
		GithubAPIBase,
		repoOwner,
		repo,
		repoOwner,
		baseRevision,
		repoOwner,
		currentCommit.SHA)

	resp, err := tryGithubGet(oauthToken, url)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		errMsg := fmt.Sprintf("error getting merge base commit response for url, %v: %v", url, err)
		grip.Error(errMsg)
		return "", APIResponseError{errMsg}
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", ResponseReadError{err.Error()}
	}
	if resp.StatusCode != http.StatusOK {
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return "", APIRequestError{Message: string(respBody)}
		}
		return "", requestError
	}
	compareResponse := &GitHubCompareResponse{}
	if err = json.Unmarshal(respBody, compareResponse); err != nil {
		return "", APIUnmarshalError{string(respBody), err.Error()}
	}
	return compareResponse.MergeBaseCommit.SHA, nil
}

func GetCommitEvent(oauthToken, repoOwner, repo, githash string) (*CommitEvent, error) {
	repoID := fmt.Sprintf("%s/%s", repoOwner, repo)
	commitURL := fmt.Sprintf("%s/repos/%s/commits/%s",
		GithubAPIBase, repoID, githash)

	grip.Info(message.Fields{
		"message": "requesting commit from github",
		"commit":  githash,
		"repo":    repoID,
		"url":     commitURL,
	})

	resp, err := tryGithubGet(oauthToken, commitURL)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		err = errors.Wrapf(err, "problem querying repo %s for %s", repoID, githash)
		grip.Error(message.Fields{
			"commit":  githash,
			"repo":    repoID,
			"url":     commitURL,
			"error":   errors.Cause(err),
			"message": "problem querying repo",
		})
		return nil, APIResponseError{err.Error()}
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}
	grip.Debug(message.Fields{
		"operation": "github api query",
		"size":      len(respBody),
		"status":    resp.Status,
		"commit":    githash,
		"repo":      repoID,
		"url":       commitURL,
	})

	if resp.StatusCode != http.StatusOK {
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
	}

	commitEvent := &CommitEvent{}
	if err = json.Unmarshal(respBody, commitEvent); err != nil {
		return nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return commitEvent, nil
}

// GetBranchEvent gets the head of the a given branch via an API call to GitHub
func GetBranchEvent(oauthToken, repoOwner, repo, branch string) (*BranchEvent,
	error) {
	branchURL := fmt.Sprintf("%v/repos/%v/%v/branches/%v",
		GithubAPIBase,
		repoOwner,
		repo,
		branch,
	)

	grip.Errorln("requesting github commit from url:", branchURL)

	resp, err := tryGithubGet(oauthToken, branchURL)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		errMsg := fmt.Sprintf("error querying '%v': %v", branchURL, err)
		grip.Error(errMsg)
		return nil, APIResponseError{errMsg}
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}
	grip.Debugf("Github API response: %s. %d bytes", resp.Status, len(respBody))

	if resp.StatusCode != http.StatusOK {
		requestError := APIRequestError{}
		if err = json.Unmarshal(respBody, &requestError); err != nil {
			return nil, APIRequestError{Message: string(respBody)}
		}
		return nil, requestError
	}

	branchEvent := &BranchEvent{}
	if err = json.Unmarshal(respBody, branchEvent); err != nil {
		return nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return branchEvent, nil
}

// githubRequest performs the specified http request. If the oauth token field is empty it will not use oauth
func githubRequest(method string, url string, oauthToken string, data interface{}) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

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
	client := &http.Client{}
	return client.Do(req)
}

func tryGithubGet(oauthToken, url string) (resp *http.Response, err error) {
	grip.Debugf("Attempting GitHub API call at '%s'", url)
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err = githubRequest("GET", url, oauthToken, nil)
			if err != nil {
				grip.Errorf("failed trying to call github GET on %s: %+v", url, err)
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusUnauthorized {
				err = errors.Errorf("Calling github GET on %v failed: got 'unauthorized' response", url)
				grip.Error(err)
				return err
			}
			if resp.StatusCode != http.StatusOK {
				err = errors.Errorf("Calling github GET on %v got a bad response code: %v", url, resp.StatusCode)
			}
			// read the results
			rateMessage, _ := getGithubRateLimit(resp.Header)
			grip.Debugf("Github API response: %s. %s", resp.Status, rateMessage)
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if err != nil {
		// couldn't get it
		if retryFail {
			grip.Errorf("Github GET on %v used up all retries.", err)
		}
		return nil, errors.WithStack(err)
	}

	return
}

// tryGithubPost posts the data to the Github api endpoint with the url given
func tryGithubPost(url string, oauthToken string, data interface{}) (resp *http.Response, err error) {
	grip.Errorf("Attempting GitHub API POST at ‘%s’", url)
	retriableGet := util.RetriableFunc(
		func() (retryError error) {
			resp, err = githubRequest("POST", url, oauthToken, data)
			if err != nil {
				grip.Errorf("failed trying to call github POST on %s: %+v", url, err)
				return util.RetriableError{err}
			}
			if resp.StatusCode == http.StatusUnauthorized {
				err = errors.Errorf("Calling github POST on %v failed: got 'unauthorized' response", url)
				grip.Error(err)
				return err
			}
			if resp.StatusCode != http.StatusOK {
				err = errors.Errorf("Calling github POST on %v got a bad response code: %v", url, resp.StatusCode)
			}
			// read the results
			rateMessage, loglevel := getGithubRateLimit(resp.Header)

			grip.Logf(loglevel, "Github API response: %v. %v", resp.Status, rateMessage)
			return nil
		},
	)

	retryFail, err := util.Retry(retriableGet, NumGithubRetries, GithubSleepTimeSecs*time.Second)
	if err != nil {
		// couldn't post it
		if retryFail {
			grip.Errorf("Github POST to '%s' used up all retries.", url)
		}
		return nil, errors.WithStack(err)
	}

	return
}

// GetGithubFileURL returns a URL that locates a github file given the owner,
// repo,remote path and revision
func GetGithubFileURL(owner, repo, remotePath, revision string) string {
	return fmt.Sprintf("https://api.github.com/repos/%v/%v/contents/%v?ref=%v",
		owner,
		repo,
		remotePath,
		revision,
	)
}

// NextPageLink returns the link to the next page for a given header's "Link"
// key based on http://developer.github.com/v3/#pagination
// For full details see http://tools.ietf.org/html/rfc5988
func NextGithubPageLink(header http.Header) string {
	hlink, ok := header["Link"]
	if !ok {
		return ""
	}

	for _, s := range hlink {
		ix := strings.Index(s, `; rel="next"`)
		if ix > -1 {
			t := s[:ix]
			op := strings.Index(t, "<")
			po := strings.Index(t, ">")
			u := t[op+1 : po]
			return u
		}
	}
	return ""
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
func GithubAuthenticate(code, clientId, clientSecret string) (githubResponse *GithubAuthResponse, err error) {
	accessUrl := "https://github.com/login/oauth/access_token"
	authParameters := GithubAuthParameters{
		ClientId:     clientId,
		ClientSecret: clientSecret,
		Code:         code,
	}
	resp, err := tryGithubPost(accessUrl, "", authParameters)
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

// GetGithubUser does a GET from GitHub for the user, email, and organizations information and
// returns the GithubLoginUser and its associated GithubOrganizations after authentication
func GetGithubUser(token string) (githubUser *GithubLoginUser, githubOrganizations []GithubOrganization, err error) {
	userUrl := fmt.Sprintf("%v/user", GithubAPIBase)
	orgUrl := fmt.Sprintf("%v/user/orgs", GithubAPIBase)
	t := fmt.Sprintf("token %v", token)
	// get the user
	resp, err := tryGithubGet(t, userUrl)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, ResponseReadError{err.Error()}
	}

	grip.Debugf("Github API response: %s. %d bytes", resp.Status, len(respBody))

	if err = json.Unmarshal(respBody, &githubUser); err != nil {
		return nil, nil, APIUnmarshalError{string(respBody), err.Error()}
	}

	// get the user's organizations
	resp, err = tryGithubGet(t, orgUrl)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, nil, errors.Wrapf(err, "Could not get user from token")
	}
	respBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, ResponseReadError{err.Error()}
	}

	grip.Debugf("Github API response: %s. %d bytes", resp.Status, len(respBody))

	if err = json.Unmarshal(respBody, &githubOrganizations); err != nil {
		return nil, nil, APIUnmarshalError{string(respBody), err.Error()}
	}
	return
}

// verifyGithubAPILimitHeader parses a Github API header to find the number of requests remaining
func verifyGithubAPILimitHeader(header http.Header) (int64, error) {
	h := (map[string][]string)(header)
	limStr, okLim := h["X-Ratelimit-Limit"]
	remStr, okRem := h["X-Ratelimit-Remaining"]

	if !okLim || !okRem || len(limStr) == 0 || len(remStr) == 0 {
		return 0, errors.New("Could not get rate limit data")
	}

	rem, err := strconv.ParseInt(remStr[0], 10, 0)
	if err != nil {
		return 0, errors.Errorf("Could not parse rate limit data: limit=%q, rate=%t", limStr, okLim)
	}

	return rem, nil
}

// CheckGithubAPILimit queries Github for the number of API requests remaining
func CheckGithubAPILimit(token string) (int64, error) {
	url := fmt.Sprintf("%v/rate_limit", GithubAPIBase)
	resp, err := githubRequest("GET", url, token, nil)
	if err != nil {
		grip.Errorf("github GET rate limit failed on %s: %+v", url, err)
		return 0, err
	}
	rem, err := verifyGithubAPILimitHeader(resp.Header)
	if err != nil {
		grip.Errorf("Error getting rate limit: %s", err)
		return 0, err
	}
	return rem, nil
}
