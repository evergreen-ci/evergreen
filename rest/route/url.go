package route

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

type requestHostKey int

const requestHostCtxKey requestHostKey = 0

func withRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, requestHostCtxKey, host)
}

func requestHost(ctx context.Context) string {
	host, _ := ctx.Value(requestHostCtxKey).(string)
	return host
}

// NewRequestHostMiddleware stores the request Host header on the context so GetURL can
// determine whether to return the corp or non-corp Evergreen UI URL.
func NewRequestHostMiddleware() gimlet.Middleware {
	return gimlet.WrapperMiddleware(setRequestHost)
}

func setRequestHost(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := withRequestHost(r.Context(), r.Host)
		next(w, r.WithContext(ctx))
	}
}

func urlHostname(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return hostFromRequestHost(strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://"))
	}
	return hostFromRequestHost(u.Host)
}

func hostFromRequestHost(host string) string {
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isCorpRequest(ctx context.Context, corpURL string) bool {
	reqHost := hostFromRequestHost(requestHost(ctx))
	if reqHost == "" {
		return false
	}
	return reqHost == urlHostname(corpURL)
}

// GetURL returns the Evergreen UI base URL that matches how the request was routed.
// Requests to the corp host receive settings.Api.CorpURL; all others receive settings.Ui.Url.
func GetURL(ctx context.Context) string {
	settings := evergreen.GetEnvironment().Settings()
	corpURL := strings.TrimRight(settings.Api.CorpURL, "/")
	nonCorpURL := strings.TrimRight(settings.Ui.Url, "/")

	if isCorpRequest(ctx, corpURL) && corpURL != "" {
		return corpURL
	}
	if nonCorpURL != "" {
		return nonCorpURL
	}
	if host := requestHost(ctx); host != "" {
		return util.HttpsUrl(host)
	}
	return nonCorpURL
}
