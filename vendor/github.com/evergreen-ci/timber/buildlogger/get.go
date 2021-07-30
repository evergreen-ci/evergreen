package buildlogger

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/peterhellberg/link"
	"github.com/pkg/errors"
)

// BuildloggerGetOptions specify the required and optional information to
// create the buildlogger HTTP GET request to cedar.
type BuildloggerGetOptions struct {
	CedarOpts timber.GetOptions

	// Request information. See cedar's REST documentation for more
	// information:
	// `https://github.com/evergreen-ci/cedar/wiki/Rest-V1-Usage`.
	ID            string
	TaskID        string
	TestName      string
	GroupID       string
	Execution     int
	Start         time.Time
	End           time.Time
	ProcessName   string
	Tags          []string
	PrintTime     bool
	PrintPriority bool
	Tail          int
	Limit         int
	Meta          bool
}

// Validate ensures BuildloggerGetOptions is configured correctly.
func (opts *BuildloggerGetOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(opts.CedarOpts.Validate())
	catcher.AddWhen(opts.ID == "" && opts.TaskID == "", errors.New("must provide an id or task id"))
	catcher.AddWhen(opts.ID != "" && opts.TaskID != "", errors.New("cannot provide both id and task id"))
	catcher.AddWhen(opts.TestName != "" && opts.TaskID == "", errors.New("must provide a task id when a test name is specified"))
	catcher.AddWhen(opts.GroupID != "" && opts.TaskID == "", errors.New("must provide a task id when a group id is specified"))
	catcher.AddWhen(opts.GroupID != "" && opts.Meta, errors.New("cannot specify a group id and set meta to true"))

	return catcher.Resolve()
}

// GetLogs returns a ReadCloser with the logs or log metadata requested via
// HTTP to a cedar service.
func GetLogs(ctx context.Context, opts BuildloggerGetOptions) (io.ReadCloser, error) {
	urlString, err := opts.parse()
	if err != nil {
		return nil, errors.Wrap(err, "problem parsing options")
	}

	resp, err := opts.CedarOpts.DoReq(ctx, urlString)
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			return &paginatedReadCloser{
				ctx:        ctx,
				header:     resp.Header,
				opts:       opts.CedarOpts,
				ReadCloser: resp.Body,
			}, nil
		}
		return nil, errors.Errorf("failed to fetch logs with resp '%s'", resp.Status)
	}
	return nil, errors.Wrapf(err, "fetch logs request failed")
}

func (opts *BuildloggerGetOptions) parse() (string, error) {
	if err := opts.Validate(); err != nil {
		return "", errors.WithStack(err)
	}

	params := fmt.Sprintf(
		"?execution=%d&proc_name=%s&print_time=%v&print_priority=%v&n=%d&limit=%d&paginate=true",
		opts.Execution,
		url.QueryEscape(opts.ProcessName),
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
		params += fmt.Sprintf("&tags=%s", url.QueryEscape(tag))
	}

	urlString := fmt.Sprintf("%s/rest/v1/buildlogger", opts.CedarOpts.BaseURL)
	if opts.ID != "" {
		urlString += fmt.Sprintf("/%s", url.PathEscape(opts.ID))
	} else if opts.TestName != "" {
		urlString += fmt.Sprintf("/test_name/%s/%s", url.PathEscape(opts.TaskID), url.PathEscape(opts.TestName))
	} else {
		urlString += fmt.Sprintf("/task_id/%s", url.PathEscape(opts.TaskID))
	}
	if opts.GroupID != "" {
		urlString += fmt.Sprintf("/group/%s", url.PathEscape(opts.GroupID))
	} else if opts.Meta {
		urlString += "/meta"
	}
	urlString += params

	return urlString, nil
}

type paginatedReadCloser struct {
	ctx    context.Context
	header http.Header
	opts   timber.GetOptions

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
		if err == io.EOF {
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
		resp, err := r.opts.DoReq(r.ctx, group.URI)
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
