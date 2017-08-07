package client

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen/rest/route"
	"golang.org/x/net/context"
)

// paginatorHelper is a struct to handle paginated GET requests
type paginatorHelper struct {
	more         bool
	routeInfo    *requestInfo
	comm         *communicatorImpl
	nextPagePath string
}

// NewpaginatorHelper creates a new paginatorHelper
func newPaginatorHelper(info *requestInfo, comm *communicatorImpl) (*paginatorHelper, error) {
	if info == nil {
		return nil, errors.New("request info cannot be nil")
	}
	if comm == nil {
		return nil, errors.New("communicator cannot be nil")
	}

	return &paginatorHelper{
		more:         true,
		routeInfo:    info,
		comm:         comm,
		nextPagePath: "",
	}, nil
}

// HasMore returns if there are more pages of data to return
func (p *paginatorHelper) hasMore() bool {
	return p.more
}

// GetNextPagePath returns the path to the next page of results, if there are more
func (p *paginatorHelper) getNextPagePath() string {
	return p.nextPagePath
}

// SetNextPagePath will set the page to start at
func (p *paginatorHelper) setNextPagePath(path string) {
	p.nextPagePath = path
}

// GetNextPage returns the response from a single page. The caller must close the
// response body
func (p *paginatorHelper) getNextPage(ctx context.Context) (*http.Response, error) {
	info := *p.routeInfo
	if p.nextPagePath != "" {
		info.path = p.nextPagePath
	}

	resp, err := p.comm.request(ctx, info, nil)
	if err != nil {
		p.more = false
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.more = false
		return nil, errors.New(resp.Status)
	}

	link := parseLink(resp.Header.Get(route.NextPageHeaderKey), string(p.routeInfo.version))
	if link != "" {
		p.nextPagePath = link
		p.more = true
	} else {
		p.nextPagePath = ""
		p.more = false
	}

	return resp, nil
}

// parseLink extracts the path of a link string returned by paginated routes
func parseLink(in, version string) string {
	regex := regexp.MustCompile(strings.Replace(version, "/", `\/`, -1) + `.+[>][;]`)
	url := regex.FindString(in)
	url = strings.TrimRight(url, ">;")
	path := strings.Replace(url, version, "", 1)

	return path
}
