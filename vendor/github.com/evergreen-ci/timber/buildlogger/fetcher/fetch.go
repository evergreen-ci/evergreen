package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/peterhellberg/link"
	"github.com/pkg/errors"
)

// GetOptions specify the required and optional information to create the
// buildlogger HTTP GET request to cedar.
type GetOptions struct {
	// The cedar service's base HTTP URL for the request.
	BaseURL string
	// The user cookie for cedar authorization. Optional.
	Cookie *http.Cookie
	// User API key and name for request header.
	UserKey  string
	UserName string
	// Request information. See cedar's REST documentation for more
	// information:
	// `https://github.com/evergreen-ci/cedar/wiki/Rest-V1-Usage`.
	ID            string
	TaskID        string
	TestName      string
	GroupID       string
	Start         time.Time
	End           time.Time
	Execution     int
	ProcessName   string
	Tags          []string
	PrintTime     bool
	PrintPriority bool
	Tail          int
	Limit         int
	Meta          bool
}

// Logs returns a ReadCloser with the logs or log metadata requested via HTTP
// to a cedar service.
func Logs(ctx context.Context, opts GetOptions) (io.ReadCloser, error) {
	url, err := opts.parse()
	if err != nil {
		return nil, errors.Wrap(err, "problem parsing options")
	}

	resp, err := doReq(ctx, url, opts)
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			return &paginatedReadCloser{
				ctx:        ctx,
				header:     resp.Header,
				opts:       opts,
				ReadCloser: resp.Body,
			}, nil
		}
		return nil, errors.Errorf("failed to fetch logs with resp '%s'", resp.Status)
	}
	return nil, errors.Wrapf(err, "fetch logs request failed")
}

func (opts GetOptions) parse() (string, error) {
	if opts.BaseURL == "" {
		return "", errors.New("must provide a base URL")
	}
	if opts.ID != "" && opts.TaskID != "" {
		return "", errors.New("cannot provide both ID and TaskID")
	}
	if opts.ID == "" && opts.TaskID == "" {
		return "", errors.New("must provide either ID or TaskID")
	}

	params := fmt.Sprintf(
		"?execution=%d&proc_name=%s&print_time=%v&print_priority=%v&n=%d&limit=%d&paginate=true",
		opts.Execution,
		opts.ProcessName,
		opts.PrintTime,
		opts.PrintPriority,
		opts.Tail,
		opts.Limit,
	)
	if !opts.Start.IsZero() {
		params += fmt.Sprintf("&start=%s", opts.Start.Format(time.RFC3339))
	}
	if !opts.End.IsZero() {
		params += fmt.Sprintf("&end=%s", opts.End.Format(time.RFC3339))
	}
	for _, tag := range opts.Tags {
		params += fmt.Sprintf("&tags=%s", tag)
	}

	url := fmt.Sprintf("%s/rest/v1/buildlogger", opts.BaseURL)
	if opts.ID != "" {
		url += fmt.Sprintf("/%s", opts.ID)
	} else {
		if opts.TestName != "" {
			url += fmt.Sprintf("/test_name/%s/%s", opts.TaskID, opts.TestName)
			if opts.GroupID != "" {
				url += fmt.Sprintf("/group/%s", opts.GroupID)
			}
		} else {
			url += fmt.Sprintf("/task_id/%s", opts.TaskID)
		}
	}

	if opts.Meta {
		url += "/meta"
	}
	url += params

	return url, nil
}

type paginatedReadCloser struct {
	ctx    context.Context
	header http.Header
	opts   GetOptions

	io.ReadCloser
}

func (r *paginatedReadCloser) Read(p []byte) (int, error) {
	if r.ReadCloser == nil {
		return 0, io.EOF
	}

	var (
		n      int
		offset int
		err    error
	)
	for offset < len(p) {
		n, err = r.ReadCloser.Read(p[offset:])
		offset += n
		if err == io.EOF && n > 0 {
			err = r.getNextPage()
		}
		if err != nil {
			break
		}
	}

	return offset, err
}

func (r *paginatedReadCloser) getNextPage() error {
	group, ok := link.ParseHeader(r.header)["next"]
	if ok {
		resp, err := doReq(r.ctx, group.URI, r.opts)
		if err != nil {
			return errors.Wrap(err, "problem requesting next page")
		}

		if err = r.Close(); err != nil {
			return errors.Wrap(err, "problem closing last response reader")
		}

		r.header = resp.Header
		r.ReadCloser = resp.Body
	} else {
		return io.EOF
	}

	return nil
}

func doReq(ctx context.Context, url string, opts GetOptions) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating http request for cedar buildlogger")
	}
	if opts.Cookie != nil {
		req.AddCookie(opts.Cookie)
	}
	if opts.UserKey != "" && opts.UserName != "" {
		req.Header.Set("Evergreen-Api-Key", opts.UserKey)
		req.Header.Set("Evergreen-Api-User", opts.UserName)
	}
	req = req.WithContext(ctx)

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	return c.Do(req)
}
