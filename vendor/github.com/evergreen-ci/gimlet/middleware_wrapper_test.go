package gimlet

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMiddlewareWrapper(t *testing.T) {
	assert := assert.New(t)

	legacyCalls := 0
	legacyFunc := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			legacyCalls++

			h(w, r)
		}
	}

	nextCalls := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
	}

	wrapped := WrapperMiddleware(legacyFunc)
	assert.Implements((*Middleware)(nil), wrapped)

	wrapped.ServeHTTP(nil, nil, next)
	assert.Equal(1, legacyCalls)
	assert.Equal(1, nextCalls)
}
