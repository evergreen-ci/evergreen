package route

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
)

var linkMatcher = regexp.MustCompile(`^\<(\S+)\>; rel=\"(\S+)\"`)

// PaginationExecutor is a struct that handles gathering necessary
// information for pagination and handles executing the pagination. It is
// designed to be embeded into a request handler to completely handler the
// execution of endpoints with pagination.
type PaginationExecutor struct {
	// KeyQueryParam is the query param that a PaginationExecutor
	// expects to hold the key to use for fetching results.
	KeyQueryParam string
	// LimitQueryParam is the query param that a PaginationExecutor
	// expects to hold the limit to use for fetching results.
	LimitQueryParam string

	// Paginator is the function that a PaginationExector uses to
	// retrieve results from the service layer.
	Paginator PaginatorFunc

	limit int
	key   string
}

// PaginationMetadata is a struct that contains all of the information for
// creating the next and previous pages of data.
type PaginationMetadata struct {
	Pages *PageResult

	KeyQueryParam   string
	LimitQueryParam string
}

// Page contains the information about a single page of the resource.
type Page struct {
	Relation string
	Key      string
	Limit    int
}

// PageResult is a type that holds the two pages that pagintion handlers create
type PageResult struct {
	Next *Page
	Prev *Page
}

// PaginatorFunc is a function that handles fetching results from the service
// layer. It takes as parameters a string which is the key to fetch starting
// from, and an int as the number of results to limit to.
type PaginatorFunc func(string, int, servicecontext.ServiceContext) ([]model.Model, *PageResult, error)

// Execute serves as an implementation of the RequestHandler's 'Execute' method.
// It calls the embeded PaginationFunc and then processes and returns the results.
func (pe *PaginationExecutor) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	models, pages, err := pe.Paginator(pe.key, pe.limit, sc)
	if err != nil {
		return ResponseData{}, err
	}

	pm := PaginationMetadata{
		Pages:           pages,
		KeyQueryParam:   pe.KeyQueryParam,
		LimitQueryParam: pe.LimitQueryParam,
	}

	rd := ResponseData{
		Result:   models,
		Metadata: &pm,
	}
	return rd, nil
}

// fetchPaginationParams gets the key and limit from the request
// and sets them on the PaginationExecutor.
func (pe *PaginationExecutor) fetchPaginationParams(r *http.Request) error {
	vals := r.URL.Query()
	if k, ok := vals[pe.KeyQueryParam]; ok && len(k) > 0 {
		pe.key = k[0]
	}

	limit := ""
	if l, ok := vals[pe.LimitQueryParam]; ok && len(l) > 0 {
		limit = l[0]
	}

	// not having a limit is not an error
	if limit == "" {
		return nil
	}
	var err error
	pe.limit, err = strconv.Atoi(limit)
	if err != nil {
		return apiv3.APIError{
			StatusCode: http.StatusBadRequest,
			Message: fmt.Sprintf("Value '%v' provided for '%v' must be integer",
				limit, pe.LimitQueryParam),
		}
	}
	return nil
}

// buildLink creates the link string for a given page of the resource.
func (p *Page) buildLink(keyQueryParam, limitQueryParam string,
	baseURL url.URL) string {

	q := baseURL.Query()
	q.Set(keyQueryParam, p.Key)
	if p.Limit != 0 {
		q.Set(limitQueryParam, fmt.Sprintf("%d", p.Limit))
	}
	baseURL.RawQuery = q.Encode()
	return fmt.Sprintf("<%s>; rel=\"%s\"", baseURL.String(), p.Relation)
}

// ParsePaginationHeader creates a PaginationMetadata using the header
// that a paginator creates.
func ParsePaginationHeader(header, keyQueryParam,
	limitQueryParam string) (*PaginationMetadata, error) {

	pm := PaginationMetadata{
		KeyQueryParam:   keyQueryParam,
		LimitQueryParam: limitQueryParam,

		Pages: &PageResult{},
	}

	scanner := bufio.NewScanner(strings.NewReader(header))

	// Looks through the lines of the header and creates a new page for each
	// link it describes.
	for scanner.Scan() {
		matches := linkMatcher.FindStringSubmatch(scanner.Text())
		if len(matches) != 3 {
			return nil, fmt.Errorf("malformed link header %v", scanner.Text())
		}
		u, err := url.Parse(matches[1])
		if err != nil {
			return nil, err
		}
		vals := u.Query()
		p := Page{}
		p.Relation = matches[2]
		if len(vals[limitQueryParam]) > 0 {
			var err error
			p.Limit, err = strconv.Atoi(vals[limitQueryParam][0])
			if err != nil {
				return nil, err
			}
		}
		if len(vals[keyQueryParam]) < 1 {
			return nil, fmt.Errorf("key query paramater must be set")
		}
		p.Key = vals[keyQueryParam][0]
		switch p.Relation {
		case "next":
			pm.Pages.Next = &p
		case "prev":
			pm.Pages.Prev = &p
		default:
			return nil, fmt.Errorf("unknown relation: %v", p.Relation)
		}

	}
	return &pm, nil
}

// MakeHeader builds a list of links to different pages of the same resource
// and writes out the result to the ResponseWriter.
// As per the specification, the header has the form of:
// Link:
//		http://evergreen.mongodb.com/{route}?key={key}&limit={limit}; rel="{relation}"
//    http://...
func (pm *PaginationMetadata) MakeHeader(w http.ResponseWriter,
	prefix, apiURL, route string, version int) error {

	//Not exactly sure what to do in this case
	if pm.Pages == nil || (pm.Pages.Next == nil && pm.Pages.Prev == nil) {
		return nil
	}
	baseURL := url.URL{
		Scheme: "http",
		Host:   apiURL,
		Path:   path.Clean(fmt.Sprintf("/%s/v%d/%s", prefix, version, route)),
	}

	b := bytes.Buffer{}
	if pm.Pages.Next != nil {
		pageLink := pm.Pages.Next.buildLink(pm.KeyQueryParam, pm.LimitQueryParam,
			baseURL)
		_, err := b.WriteString(pageLink)
		if err != nil {
			return err
		}
	}
	if pm.Pages.Prev != nil {
		if pm.Pages.Next != nil {
			_, err := b.WriteString("\n")
			if err != nil {
				return err
			}
		}
		pageLink := pm.Pages.Prev.buildLink(pm.KeyQueryParam, pm.LimitQueryParam,
			baseURL)
		_, err := b.WriteString(pageLink)
		if err != nil {
			return err
		}
	}
	w.Header().Set("Link", b.String())
	return nil
}
