package evergreen

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRESTClientAuthMechanismFromRequest(t *testing.T) {
	t.Run("BearerTokenIsOAuth", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "Bearer token")
		assert.Equal(t, "oauth", RESTClientAuthMechanismFromRequest(r))
	})
	t.Run("BearerCaseInsensitive", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "bearer token")
		assert.Equal(t, "oauth", RESTClientAuthMechanismFromRequest(r))
	})
	t.Run("ApiUserOrKeyIsStatic", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r.Header.Set(APIUserHeader, "user")
		assert.Equal(t, "api_key", RESTClientAuthMechanismFromRequest(r))

		r2, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r2.Header.Set(APIKeyHeader, "key")
		assert.Equal(t, "api_key", RESTClientAuthMechanismFromRequest(r2))
	})
	t.Run("BearerPreferredWhenBothSent", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r.Header.Set(AuthorizationHeader, "Bearer t")
		r.Header.Set(APIUserHeader, "u")
		r.Header.Set(APIKeyHeader, "k")
		assert.Equal(t, "oauth", RESTClientAuthMechanismFromRequest(r))
	})
	t.Run("NoHeadersReturnsEmpty", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		assert.Equal(t, "", RESTClientAuthMechanismFromRequest(r))
	})
}
