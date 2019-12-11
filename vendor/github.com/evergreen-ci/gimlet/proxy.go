package gimlet

import (
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

// ProxyOptions describes a simple reverse proxy service that can be
// the handler for a route in an application. The proxy implementation
// can modify the headers of the request. Requests are delgated to
// backends in the target pool
type ProxyOptions struct {
	HeadersToDelete   []string
	HeadersToAdd      map[string]string
	RemotePrefix      string
	StripSourcePrefix bool
	TargetPool        []string
	FindTarget        func(*url.URL) []string
}

// Validate checks the default configuration of a proxy configuration.
func (opts *ProxyOptions) Validate() error {
	if !strings.HasPrefix(opts.RemotePrefix, "/") {
		opts.RemotePrefix = "/" + opts.RemotePrefix
	}

	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(len(opts.TargetPool) == 0 && opts.FindTarget == nil, "must specify a way to resolve target host")
	catcher.NewWhen(len(opts.TargetPool) >= 1 && opts.FindTarget != nil, "cannot specify more than one target resolution option")
	return catcher.Resolve()
}

func (opts *ProxyOptions) getHost() string { return getRandomHost(opts.TargetPool) }

func getRandomHost(hosts []string) string {
	hostLen := len(hosts)
	switch {
	case hostLen == 1:
		return hosts[0]
	case hostLen > 1:
		return hosts[rand.Intn(hostLen)]
	default:
		return ""
	}
}

func (opts *ProxyOptions) resolveTarget(u *url.URL) string {
	if opts.FindTarget == nil {
		return ""
	}

	return getRandomHost(opts.FindTarget(u))
}

func (opts *ProxyOptions) director(req *http.Request) {
	for k, v := range opts.HeadersToAdd {
		req.Header.Add(k, v)
	}

	for _, k := range opts.HeadersToDelete {
		req.Header.Del(k)
	}

	if _, ok := req.Header["User-Agent"]; !ok {
		// explicitly disable User-Agent so it's not set to default value
		req.Header.Set("User-Agent", "")
	}

	if target := opts.getHost(); target != "" {
		req.URL.Host = target
	} else if target := opts.resolveTarget(req.URL); target != "" {
		req.URL.Host = target
	} else {
		panic("could not resolve proxy target host")
	}

	if opts.StripSourcePrefix {
		req.URL.Path = "/"
	}

	req.URL.Path = singleJoiningSlash(opts.RemotePrefix, req.URL.Path)
}

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

	r.handler = (&httputil.ReverseProxy{
		ErrorLog: grip.MakeStandardLogger(level.Warning),
		Director: opts.director,
	}).ServeHTTP

	return r
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
