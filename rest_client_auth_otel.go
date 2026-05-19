package evergreen

import (
	"net/http"
	"strings"
)

// RESTClientAuthOtelAttribute is set on HTTP server spans when the client sends programmatic REST credentials.
const RESTClientAuthOtelAttribute = "evergreen.http.rest_client_auth"

// RESTClientAuthMechanismFromRequest classifies Evergreen REST client auth headers for tracing.
// It returns "oauth" for Authorization Bearer tokens, "api_key" when Api-User or Api-Key is set,
// or "" when neither pattern is present (e.g. cookie session or unauthenticated).
func RESTClientAuthMechanismFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get(AuthorizationHeader))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return "oauth"
	}
	if r.Header.Get(APIUserHeader) != "" || r.Header.Get(APIKeyHeader) != "" {
		return "api_key"
	}
	return ""
}
