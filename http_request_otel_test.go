package evergreen

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestHTTPClientAuthMechanismFromRequest(t *testing.T) {
	t.Run("BearerTokenIsOAuth", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "Bearer token")
		assert.Equal(t, "oauth", httpClientAuthMechanismFromRequest(r))
	})
	t.Run("BearerCaseInsensitive", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "bearer token")
		assert.Equal(t, "oauth", httpClientAuthMechanismFromRequest(r))
	})
	t.Run("ApiUserOrKeyIsStatic", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(APIUserHeader, "user")
		assert.Equal(t, "api_key", httpClientAuthMechanismFromRequest(r))

		r2, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r2.Header.Set(APIKeyHeader, "key")
		assert.Equal(t, "api_key", httpClientAuthMechanismFromRequest(r2))
	})
	t.Run("BearerPreferredWhenBothSent", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "Bearer t")
		r.Header.Set(APIUserHeader, "u")
		r.Header.Set(APIKeyHeader, "k")
		assert.Equal(t, "oauth", httpClientAuthMechanismFromRequest(r))
	})
	t.Run("NoHeadersReturnsEmpty", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		assert.Equal(t, "", httpClientAuthMechanismFromRequest(r))
	})
}

func TestHTTPRequestOtelAttributes(t *testing.T) {
	t.Run("NoAuthHeadersAndNoUserReturnsSameRequest", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		assert.Same(t, r, httpRequestOtelAttributes(r))
	})
	t.Run("ApiKeyHeadersOnly", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(APIUserHeader, "cli-user")
		assert.Equal(t, []attribute.KeyValue{
			attribute.String(HTTPClientAuthOtelAttribute, "api_key"),
		}, otelAttributesOnRequest(t, r))
	})
	t.Run("AuthenticatedHumanUser", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		humanOpts, err := gimlet.NewBasicUserOptions("human")
		require.NoError(t, err)
		r = r.WithContext(gimlet.AttachUser(r.Context(), gimlet.NewBasicUser(humanOpts)))
		assert.Equal(t, []attribute.KeyValue{
			attribute.Bool(HTTPUserOnlyAPIOtelAttribute, false),
		}, otelAttributesOnRequest(t, r))
	})
	t.Run("AuthenticatedAPIOnlyUser", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		serviceOpts, err := gimlet.NewBasicUserOptions("service")
		require.NoError(t, err)
		r = r.WithContext(gimlet.AttachUser(r.Context(), gimlet.NewBasicUser(serviceOpts.APIOnly(true))))
		assert.Equal(t, []attribute.KeyValue{
			attribute.Bool(HTTPUserOnlyAPIOtelAttribute, true),
		}, otelAttributesOnRequest(t, r))
	})
	t.Run("OAuthHeaderWithAuthenticatedUser", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		require.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "Bearer token")
		oauthOpts, err := gimlet.NewBasicUserOptions("oauth-user")
		require.NoError(t, err)
		r = r.WithContext(gimlet.AttachUser(r.Context(), gimlet.NewBasicUser(oauthOpts)))
		assert.Equal(t, []attribute.KeyValue{
			attribute.String(HTTPClientAuthOtelAttribute, "oauth"),
			attribute.Bool(HTTPUserOnlyAPIOtelAttribute, false),
		}, otelAttributesOnRequest(t, r))
	})
}

// otelAttributesOnRequest records attributes from httpRequestOtelAttributes on a test span.
func otelAttributesOnRequest(t *testing.T, r *http.Request) []attribute.KeyValue {
	t.Helper()

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(utility.NewAttributeSpanProcessor()),
		sdktrace.WithSpanProcessor(spanRecorder),
	)

	annotated := httpRequestOtelAttributes(r)
	_, span := tp.Tracer("test").Start(annotated.Context(), "test")
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)
	return ended[0].Attributes()
}
