package gimlet

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMiddlewareFuncWrapper(t *testing.T) {
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
	assert.Equal(0, legacyCalls)
	assert.Equal(0, nextCalls)

	wrapped.ServeHTTP(nil, nil, next)
	assert.Equal(1, legacyCalls)
	assert.Equal(1, nextCalls)
}

func TestMiddlewareWrapper(t *testing.T) {
	assert := assert.New(t)

	legacyCalls := 0
	legacyFunc := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			legacyCalls++

			h.ServeHTTP(w, r)
		})
	}

	nextCalls := 0
	next := func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
	}

	wrapped := WrapperHandlerMiddleware(legacyFunc)
	assert.Implements((*Middleware)(nil), wrapped)
	assert.Equal(0, legacyCalls)
	assert.Equal(0, nextCalls)

	wrapped.ServeHTTP(nil, nil, next)

	assert.Equal(1, legacyCalls)
	assert.Equal(1, nextCalls)
}
