package thirdparty

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var (
	MaxRedirects = 10
)

type httpGet interface {
	doGet(string, string, string) (*http.Response, error)
}

type liveHttpGet struct{}

func shouldRedirectGet(statusCode int) bool {
	switch statusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect:
		return true
	}
	return false
}

func doFollowingRedirectsWithHeaders(client *http.Client, ireq *http.Request) (resp *http.Response, err error) {
	// Default Go HTTP client silently wipes headers on redirect, so we need to
	// write our own. See http://golang.org/src/pkg/net/http/client.go#L273
	var base *url.URL
	req := ireq
	urlStr := "" // next relative or absolute URL to fetch (after first request)
	for redirect := 0; ; redirect++ {
		if redirect != 0 {
			req = new(http.Request)
			req.Method = ireq.Method
			// This line is what Go doesn't do. Undocumented but known issue, see
			// https://groups.google.com/forum/#!topic/golang-nuts/OwGvopYXpwE
			req.Header = ireq.Header

			req.URL, err = base.Parse(urlStr)
			if err != nil {
				break
			}
		}
		urlStr = req.URL.String()
		if resp, err = client.Transport.RoundTrip(req); err != nil {
			break
		}

		if shouldRedirectGet(resp.StatusCode) {
			resp.Body.Close()
			if urlStr = resp.Header.Get("Location"); urlStr == "" {
				err = errors.New(fmt.Sprintf("%d response missing Location header", resp.StatusCode))
				break
			}

			if redirect+1 >= MaxRedirects {
				return nil, fmt.Errorf("Too many redirects")
			}

			base = req.URL
			continue
		}
		return
	}

	return
}

func (self liveHttpGet) doGet(url string, username string, password string) (*http.Response, error) {
	tr := &http.Transport{
		DisableCompression: true,
		DisableKeepAlives:  false,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "*/*")
	req.SetBasicAuth(username, password)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Transport: tr}
	var resp *http.Response
	resp, err = doFollowingRedirectsWithHeaders(client, req)
	return resp, err
}
