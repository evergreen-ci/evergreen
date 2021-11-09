package timber

import (
	"context"
	"io"
	"net/http"

	"github.com/peterhellberg/link"
	"github.com/pkg/errors"
)

type paginatedReadCloser struct {
	ctx    context.Context
	header http.Header
	opts   GetOptions

	io.ReadCloser
}

// NewPaginatedReadCloser returns an io.ReadCloser implementation for paginated
// HTTP responses from a Cedar service. It is safe to pass in a non-paginated
// response, thus the caller need not check for the appropriate header keys.
// GetOptions is used to make any subsequent page requests.
func NewPaginatedReadCloser(ctx context.Context, resp *http.Response, opts GetOptions) *paginatedReadCloser {
	return &paginatedReadCloser{
		ctx:        ctx,
		header:     resp.Header,
		opts:       opts,
		ReadCloser: resp.Body,
	}
}

// Read reads the underlying HTTP response body. Once the body is read, the
// HTTP response header is used to request the next page, if any. If a
// subsequent page is successfully requested, the previous body is closed and
// both the body and header are replaced with the new response. An io.EOF error
// is returned when the current response body is exhausted and either the
// response header does not contain a "next" key or the "next" key's URL
// returns no data.
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
			return errors.Wrap(err, "requesting next page")
		}

		if err = r.Close(); err != nil {
			return errors.Wrap(err, "closing last response reader")
		}

		r.header = resp.Header
		r.ReadCloser = resp.Body
	} else {
		return io.EOF
	}

	return nil
}
