package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvergreenCommunicatorConstructor(t *testing.T) {
	t.Run("FailsWithoutServerURL", func(t *testing.T) {
		client, err := NewCommunicator("")
		assert.Error(t, err)
		assert.Zero(t, client)
	})
	client, err := NewCommunicator("url")
	require.NoError(t, err)
	defer client.Close()

	c, ok := client.(*communicatorImpl)
	assert.True(t, ok, true)
	assert.Empty(t, c.apiUser)
	assert.Empty(t, c.apiKey)
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)

	client.SetAPIUser("apiUser")
	client.SetAPIKey("apiKey")
	assert.Equal(t, "apiUser", c.apiUser)
	assert.Equal(t, "apiKey", c.apiKey)
}

// TestOAuthTokenSourceRefreshesOnExpiry verifies that a 401 from an expired access token triggers ForceRefresh and a successful retry.
func TestOAuthTokenSourceRefreshesOnExpiry(t *testing.T) {
	var mu sync.Mutex
	validToken := "valid-token"

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		expected := validToken
		mu.Unlock()
		if r.Header.Get("Authorization") == "Bearer "+expected {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiServer.Close()

	c, err := NewCommunicator(apiServer.URL)
	require.NoError(t, err)
	defer c.Close()

	c.SetOAuth("valid-token")
	ts := &mockTokenSource{token: "valid-token"}
	ts.refreshFunc = func() string {
		mu.Lock()
		defer mu.Unlock()
		return validToken
	}
	c.SetOAuthTokenSource(ts)
	impl := c.(*communicatorImpl)
	info := requestInfo{method: http.MethodGet, path: "test"}

	resp, err := impl.request(t.Context(), info, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	mu.Lock()
	validToken = "rotated-token"
	mu.Unlock()

	resp, err = impl.request(t.Context(), info, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"TokenSource refreshes transparently on 401")
	resp.Body.Close()
	assert.True(t, ts.forceRefreshCalled)
}

// TestOAuthTokenSourceConcurrentRequestsAfterExpiry verifies that concurrent requests after token expiry all succeed.
func TestOAuthTokenSourceConcurrentRequestsAfterExpiry(t *testing.T) {
	var mu sync.Mutex
	validToken := "valid-token"

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		expected := validToken
		mu.Unlock()
		if r.Header.Get("Authorization") == "Bearer "+expected {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiServer.Close()

	c, err := NewCommunicator(apiServer.URL)
	require.NoError(t, err)
	defer c.Close()

	c.SetOAuth("valid-token")
	ts := &mockTokenSource{token: "valid-token"}
	ts.refreshFunc = func() string {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return validToken
	}
	c.SetOAuthTokenSource(ts)
	impl := c.(*communicatorImpl)
	info := requestInfo{method: http.MethodGet, path: "test"}

	mu.Lock()
	validToken = "rotated-token"
	mu.Unlock()

	const numConcurrent = 10
	var wg sync.WaitGroup
	statuses := make([]int, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := impl.request(context.Background(), info, nil)
			if err != nil {
				statuses[idx] = -1
				return
			}
			statuses[idx] = resp.StatusCode
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	for i, status := range statuses {
		assert.Equal(t, http.StatusOK, status,
			"Concurrent request %d should succeed after token refresh", i)
	}
}

type mockTokenSource struct {
	mu                 sync.Mutex
	token              string
	forceRefreshCalled bool
	refreshFunc        func() string
}

func (m *mockTokenSource) Token(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.token, nil
}

func (m *mockTokenSource) ForceRefresh(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forceRefreshCalled = true
	if m.refreshFunc != nil {
		m.token = m.refreshFunc()
	}
	return m.token, nil
}
