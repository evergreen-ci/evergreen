package thirdparty

import (
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	GithubBase          = "https://github.com"
	NumGithubRetries    = 3
	GithubSleepTimeSecs = 1
	GithubAPIBase       = "https://api.github.com/repos"
)

// GetGithubCommits returns a slice of GithubCommit objects from
// the given commitsURL when provided a valid oauth token
func GetGithubCommits(oauthToken, commitsURL string) (
	githubCommits []GithubCommit, header http.Header, err error) {
	resp, err := tryGithubGet(oauthToken, commitsURL)
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url ‘%v’", commitsURL)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, nil, APIResponseError{errMsg}
	}
	defer resp.Body.Close()
	if err != nil {
		errMsg := fmt.Sprintf("error querying ‘%v’: %v", commitsURL, err)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, nil, APIResponseError{errMsg}
	}

	header = resp.Header
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, ResponseReadError{err.Error()}
	}

	evergreen.Logger.Logf(slogger.INFO, "Github API response: %v. %v bytes",
		resp.Status, len(respBody))

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

// GetGithubFile returns a struct that contains the contents of files within
// a repository as Base64 encoded content.
func GetGithubFile(oauthToken, fileURL string) (
	githubFile *GithubFile, err error) {
	resp, err := tryGithubGet(oauthToken, fileURL)
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url ‘%v’", fileURL)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, APIResponseError{errMsg}
	}
	defer resp.Body.Close()

	if err != nil {
		errMsg := fmt.Sprintf("error querying ‘%v’: %v", fileURL, err)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, APIResponseError{errMsg}
	}

	if resp.StatusCode != http.StatusOK {
		evergreen.Logger.Errorf(slogger.ERROR, "Github API response: ‘%v’",
			resp.Status)
		if resp.StatusCode == http.StatusNotFound {
			return nil, FileNotFoundError{fileURL}
		}
		return nil, fmt.Errorf("github API returned status ‘%v’", resp.Status)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}

	evergreen.Logger.Logf(slogger.INFO, "Github API response: %v. %v bytes",
		resp.Status, len(respBody))

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

func GetCommitEvent(oauthToken, repoOwner, repo, githash string) (*CommitEvent,
	error) {
	commitURL := fmt.Sprintf("%v/%v/%v/commits/%v",
		GithubAPIBase,
		repoOwner,
		repo,
		githash,
	)

	resp, err := tryGithubGet(oauthToken, commitURL)
	if resp == nil {
		errMsg := fmt.Sprintf("nil response from url ‘%v’", commitURL)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, APIResponseError{errMsg}
	}

	defer resp.Body.Close()
	if err != nil {
		errMsg := fmt.Sprintf("error querying ‘%v’: %v", commitURL, err)
		evergreen.Logger.Logf(slogger.ERROR, errMsg)
		return nil, APIResponseError{errMsg}
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ResponseReadError{err.Error()}
	}
	evergreen.Logger.Logf(slogger.INFO, "Github API response: %v. %v bytes",
		resp.Status, len(respBody))

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

// tryGithubGet wraps githubGet in a retry block
func tryGithubGet(oauthToken, url string) (resp *http.Response, err error) {
	evergreen.Logger.Errorf(slogger.ERROR, "Attempting API call at ‘%v’...", url)
	for i := 1; i < NumGithubRetries; i++ {
		resp, err = githubGet(oauthToken, url)
		if err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Unable to make request for "+
				"‘%v’: %v", url, err.Error())
			continue
		}
		if resp != nil {
			if resp.StatusCode == http.StatusOK {
				break
			}
			evergreen.Logger.Logf(slogger.DEBUG, "Github gave a bad http response "+
				"(%v) to url %#v - sleeping for %v", resp.Status,
				url, time.Duration(GithubSleepTimeSecs*i)*time.Second)
		} else {
			evergreen.Logger.Logf(slogger.DEBUG, "Github returned a nil response at:",
				url)
		}
		time.Sleep(time.Duration(GithubSleepTimeSecs*i) * time.Second)
	}
	if resp != nil {
		header := resp.Header
		rateMessage, loglevel := getGithubRateLimit(header)
		evergreen.Logger.Logf(loglevel, "Github API response: %v. %v", resp.Status,
			rateMessage)
	}
	return
}

// githubGet queries the Github api endpoint specified by url with the given
// oauth token
func githubGet(oauthToken string, url string) (*http.Response, error) {
	if !strings.HasPrefix(oauthToken, "token ") {
		return nil, fmt.Errorf("Invalid oauth token in config: %v", oauthToken)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// add request headers
	req.Header.Add("Authorization", oauthToken)
	client := &http.Client{}
	return client.Do(req)
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
func getGithubRateLimit(header http.Header) (message string,
	loglevel slogger.Level) {
	h := (map[string][]string)(header)
	limStr, okLim := h["X-Ratelimit-Limit"]
	remStr, okRem := h["X-Ratelimit-Remaining"]

	// ensure that we were able to read the rate limit header
	if !okLim || !okRem || len(limStr) == 0 || len(remStr) == 0 {
		message, loglevel = "Could not get rate limit data", slogger.WARN
		return
	}

	// parse the rate limits
	lim, limErr := strconv.ParseInt(limStr[0], 10, 0) // parse in decimal to int
	rem, remErr := strconv.ParseInt(remStr[0], 10, 0)

	// ensure we successfully parsed the rate limits
	if limErr != nil || remErr != nil {
		message, loglevel = fmt.Sprintf("Could not parse rate limit data: "+
			"limit=%q, rate=%q", limStr, okLim), slogger.WARN
		return
	}

	// We're in good shape
	if rem > int64(0.1*float32(lim)) {
		message, loglevel = fmt.Sprintf("Rate limit: %v/%v", rem, lim),
			slogger.INFO
		return
	}

	// we're running short
	if rem > 20 {
		message, loglevel = fmt.Sprintf("Rate limit significantly low: %v/%v",
			rem, lim), slogger.WARN
		return
	}

	// we're in trouble
	message, loglevel = fmt.Sprintf("Throttling required - rate limit almost "+
		"exhausted: %v/%v", rem, lim), slogger.ERROR
	return
}
