package graphql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestMiddleware(t *testing.T) {
	t.Run("InjectsLoadersIntoContext", func(t *testing.T) {
		var capturedCtx context.Context

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		wrappedHandler := DataloaderMiddleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify loaders were injected
		loaders := DataloaderFor(capturedCtx)
		require.NotNil(t, loaders)
		require.NotNil(t, loaders.UserLoader)
		require.NotNil(t, loaders.VersionLoader)
		require.NotNil(t, loaders.TaskLoader)
	})
}

func TestNewLoaders(t *testing.T) {
	loaders := NewLoaders()
	require.NotNil(t, loaders)
	require.NotNil(t, loaders.UserLoader)
	require.NotNil(t, loaders.VersionLoader)
	require.NotNil(t, loaders.TaskLoader)
}

// setupLoaderContext creates a context with dataloaders injected.
func setupLoaderContext(ctx context.Context) context.Context {
	loaders := NewLoaders()
	return context.WithValue(ctx, loadersKey, loaders)
}
