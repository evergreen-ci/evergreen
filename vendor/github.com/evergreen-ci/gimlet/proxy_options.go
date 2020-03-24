package gimlet

import (
	"math/rand"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
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
	FindTarget        func(*http.Request) ([]string, error)
	RemoteScheme      string
	ErrorHandler      func(http.ResponseWriter, *http.Request, error)
	Transport         http.RoundTripper
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

func (opts *ProxyOptions) resolveTarget(r *http.Request) string {
	if opts.FindTarget == nil {
		return ""
	}
	target, err := opts.FindTarget(r)
	if err != nil {
		return ""
	}
	return getRandomHost(target)
}

func (opts *ProxyOptions) getPath(r *http.Request) string {
	path := r.URL.Path
	if opts.StripSourcePrefix {
		route := mux.CurrentRoute(r)
		regexString, err := route.GetPathRegexp()
		if err != nil {
			grip.Error(message.Fields{
				"message": "can't get route regexp",
				"route":   route,
				"URL":     path,
			})
			return path
		}
		compiled, err := regexp.Compile(regexString)
		if err != nil {
			grip.Error(message.Fields{
				"message": "can't strip path",
				"regex":   regexString,
				"URL":     r.URL.Path,
			})
			return path
		}
		path = compiled.ReplaceAllLiteralString(path, "")
	}

	return path
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
	} else if target := opts.resolveTarget(req); target != "" {
		req.URL.Host = target
	} else {
		panic("could not resolve proxy target host")
	}

	req.URL.Path = singleJoiningSlash(opts.RemotePrefix, opts.getPath(req))

	if opts.RemoteScheme != "" {
		req.URL.Scheme = opts.RemoteScheme
	}
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
