package gimlet

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/mongodb/grip"
)

// ResponsePages holds pagination metadata for a route built with the
// Responder interface.
//
// The pagination types and methods are
type ResponsePages struct {
	Next *Page
	Prev *Page
}

// GetLinks returns the strings for use in the links header
func (r *ResponsePages) GetLinks(route string) string {
	if strings.HasPrefix(route, "/") {
		route = route[1:]
	}

	links := []string{}

	if r.Next != nil {
		links = append(links, r.Next.GetLink(route))
	}

	if r.Prev != nil {
		links = append(links, r.Prev.GetLink(route))
	}

	return strings.Join(links, "\n")
}

// Validate checks each page, if present, and ensures that the
// pagination metadata are consistent.
func (r *ResponsePages) Validate() error {
	catcher := grip.NewCatcher()
	for _, p := range []*Page{r.Next, r.Prev} {
		if p == nil {
			continue
		}

		catcher.Add(p.Validate())
	}

	return catcher.Resolve()
}

// Page represents the metadata required to build pagination links. To
// build the page, the route must have access to the full realized
// path, including any extra query parameters, to make it possible to
// build the metadata.
type Page struct {
	BaseURL         string
	KeyQueryParam   string
	LimitQueryParam string

	Key      string
	Limit    int
	Relation string

	url *url.URL
}

// Validate ensures that the page has populated all of the required
// data. Additionally Validate parses the BaseURL, and *must* be
// called before GetLink.
func (p *Page) Validate() error {
	errs := []string{}

	if p.BaseURL == "" {
		errs = append(errs, "base url not specified")
	}

	if p.KeyQueryParam == "" {
		errs = append(errs, "key query parameter name not specified")
	}

	if p.LimitQueryParam == "" {
		errs = append(errs, "limit query parameter name not specified")
	}

	if p.Relation == "" {
		errs = append(errs, "page relation not specified")
	}

	if p.Key == "" {
		errs = append(errs, "limit not specified")
	}

	_, err := url.Parse(p.BaseURL)
	if err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// GetLink returns the pagination metadata for this page. It is called
// by the GetLinks function. Your code need not use this call
// directly in most cases, except for testing.
func (p *Page) GetLink(route string) string {
	url, err := url.Parse(fmt.Sprintf("%s/%s", p.BaseURL, route))
	if err != nil {
		grip.Alertf("encountered error '%v' building page, falling back to baseURL '%s'", err, p.BaseURL)
		url = p.url
	}

	q := url.Query()
	q.Set(p.KeyQueryParam, p.Key)

	if p.Limit != 0 {
		q.Set(p.LimitQueryParam, fmt.Sprintf("%d", p.Limit))
	}

	url.RawQuery = q.Encode()

	return fmt.Sprintf("<%s>; rel=\"%s\"", url, p.Relation)
}
