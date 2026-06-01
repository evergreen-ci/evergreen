package evergreen

import (
	"net/http"
	"strings"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"go.opentelemetry.io/otel/attribute"
)

// httpClientAuthMechanismFromRequest classifies the type of authentication used for the request.
func httpClientAuthMechanismFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get(AuthorizationHeader))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return "oauth"
	}
	if r.Header.Get(APIUserHeader) != "" || r.Header.Get(APIKeyHeader) != "" {
		return "api_key"
	}
	return ""
}

// httpRequestUserOtelAttributes appends OpenTelemetry attributes for the incoming HTTP request to its context.
func httpRequestUserOtelAttributes(r *http.Request) *http.Request {
	var attrs []attribute.KeyValue

	if mechanism := httpClientAuthMechanismFromRequest(r); mechanism != "" {
		attrs = append(attrs, attribute.String(HTTPClientAuthOtelAttribute, mechanism))
	}

	if u := gimlet.GetUser(r.Context()); u != nil {
		attrs = append(attrs, attribute.Bool(HTTPUserOnlyAPIOtelAttribute, u.IsAPIOnly()))
	}

	if len(attrs) == 0 {
		return r
	}
	return r.WithContext(utility.ContextWithAppendedAttributes(r.Context(), attrs))
}

// NewHTTPRequestOtelMiddleware returns middleware that records HTTP request tracing attributes.
// It must run after gimlet.UserMiddleware so user fields are available when the client is authenticated.
func NewHTTPRequestOtelMiddleware() gimlet.Middleware {
	return gimlet.WrapperMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) {
			r = httpRequestUserOtelAttributes(r)
			next(rw, r)
		}
	})
}
