package util

import (
	"io"
	"net/http"

	"github.com/pkg/errors"
)

const maxRequestSize = 16 * 1024 * 1024  // 16 MB
const maxResponseSize = 16 * 1024 * 1024 // 16 MB

type requestReader struct {
	req *http.Request
	*io.LimitedReader
}

// NewRequestReader returns an io.ReadCloser closer for the body of an
// *http.Request, using a limited reader internally to avoid unbounded
// reading from the request body. The reader is limited to 16 megabytes.
func NewRequestReader(req *http.Request) io.ReadCloser {
	return NewRequestReaderWithSize(req, maxRequestSize)
}

// NewRequestReaderWithSize returns an io.ReadCloser closer for the body of an
// *http.Request with a user-specified size.
func NewRequestReaderWithSize(req *http.Request, size int64) io.ReadCloser {
	return &requestReader{
		req: req,
		LimitedReader: &io.LimitedReader{
			R: req.Body,
			N: size,
		},
	}
}

func (r *requestReader) Close() error {
	return errors.WithStack(r.req.Body.Close())
}

type responseReader struct {
	req *http.Response
	*io.LimitedReader
}

// NewResponseReader returns an io.ReadCloser closer for the body of an
// *http.Response, using a limited reader internally to avoid unbounded
// reading from the request body. The reader is limited to 16 megabytes.
func NewResponseReader(req *http.Response) io.ReadCloser {
	return NewResponseReaderWithSize(req, maxResponseSize)
}

// NewResponseReaderWithSize returns an io.ReadCloser closer for the body of an
// *http.Response with a user-specified size.
func NewResponseReaderWithSize(req *http.Response, size int64) io.ReadCloser {
	return &responseReader{
		req: req,
		LimitedReader: &io.LimitedReader{
			R: req.Body,
			N: size,
		},
	}
}

func (r *responseReader) Close() error {
	return errors.WithStack(r.req.Body.Close())
}
