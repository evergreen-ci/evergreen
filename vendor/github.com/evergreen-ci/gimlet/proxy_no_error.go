// +build !go1.11

package gimlet

import (
	"net/http/httputil"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

// Proxy adds a simple reverse proxy handler to the specified route,
// based on the options described in the ProxyOption structure.
// In most cases you'll want to specify a route matching pattern
// that captures all routes that begin with a specific prefix.
func (r *APIRoute) Proxy(opts ProxyOptions) *APIRoute {
	if err := opts.Validate(); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message":          "invalid proxy options",
			"route":            r.route,
			"version":          r.version,
			"existing_handler": r.handler != nil,
		}))
		return r
	}

	if opts.ErrorHandler != nil {
		grip.Alert(message.Fields{
			"message":          "custom error handler ignored in < go1.11",
			"route":            r.route,
			"version":          r.version,
			"existing_handler": r.handler != nil,
		})
	}

	r.handler = (&httputil.ReverseProxy{
		Transport: opts.Transport,
		ErrorLog:  grip.MakeStandardLogger(level.Warning),
		Director:  opts.director,
	}).ServeHTTP

	return r
}
