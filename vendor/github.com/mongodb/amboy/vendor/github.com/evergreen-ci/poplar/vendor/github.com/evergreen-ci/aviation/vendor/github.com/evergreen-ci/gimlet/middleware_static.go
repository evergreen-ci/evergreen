package gimlet

import (
	"net/http"

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
