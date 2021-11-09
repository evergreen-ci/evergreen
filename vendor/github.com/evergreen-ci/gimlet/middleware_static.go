package gimlet

import (
	"net/http"

	"github.com/phyber/negroni-gzip/gzip"
	"github.com/urfave/negroni"
)

// NewStatic provides a convince wrapper around negroni's static file
// server middleware.
func NewStatic(prefix string, fs http.FileSystem) Middleware {
	return &negroni.Static{
		Dir:       fs,
		Prefix:    prefix,
		IndexFile: "index.html",
	}
}

// NewGzipDefault produces a gzipping middleware with default compression.
func NewGzipDefault() Middleware { return gzip.Gzip(gzip.DefaultCompression) }

// NewGzipSpeed produces a gzipping middleware with fastest compression.
func NewGzipSpeed() Middleware { return gzip.Gzip(gzip.BestSpeed) }

// NewGzipSize produces a gzipping middleware with best compression.
func NewGzipSize() Middleware { return gzip.Gzip(gzip.BestCompression) }
