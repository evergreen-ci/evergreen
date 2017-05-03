package util

import (
	"io"
	"net/http"

	"github.com/pkg/errors"
)

const maxRequestSize = 16 * 1024 * 1024 // 16 MB

type requestReader struct {
	req *http.Request
	*io.LimitedReader
}

// NewRequestReader returns an io.ReadCloser closer for the body of an
// *http.Request, using a limited reader internally to avoid unbounded
// reading from the request body. The reader is limited to 16 megabytes.
func NewRequestReader(req *http.Request) io.ReadCloser {
	return &requestReader{
		req: req,
		LimitedReader: &io.LimitedReader{
			R: req.Body,
			N: maxRequestSize,
		},
	}
}

func (r *requestReader) Close() error {
	return errors.WithStack(r.req.Body.Close())
}
