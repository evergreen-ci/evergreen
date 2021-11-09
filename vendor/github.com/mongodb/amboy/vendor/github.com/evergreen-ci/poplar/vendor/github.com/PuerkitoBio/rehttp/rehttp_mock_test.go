package rehttp

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRoundTripper struct {
	t *testing.T

	mu     sync.Mutex
	calls  int
	bodies []string
	retFn  func(int, *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()

	att := m.calls
	m.calls++
	if req.Body != nil {
		var buf bytes.Buffer
		_, err := io.Copy(&buf, req.Body)
		req.Body.Close()
		require.Nil(m.t, err)
		m.bodies = append(m.bodies, buf.String())
	}
	m.mu.Unlock()

	return m.retFn(att, req)
}

func (m *mockRoundTripper) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockRoundTripper) Bodies() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bodies
}

func TestMockClientRetry(t *testing.T) {
	retFn := func(att int, req *http.Request) (*http.Response, error) {
		return nil, tempErr{}
	}
	mock := &mockRoundTripper{t: t, retFn: retFn}

	tr := NewTransport(mock, RetryAll(RetryMaxRetries(1), RetryTemporaryErr()), ConstDelay(0))

	client := &http.Client{
		Transport: tr,
	}
	_, err := client.Get("http://example.com")
	if assert.NotNil(t, err) {
		uerr, ok := err.(*url.Error)
		require.True(t, ok)
		assert.Equal(t, tempErr{}, uerr.Err)
	}
	assert.Equal(t, 2, mock.Calls())
}

func TestMockClientFailBufferBody(t *testing.T) {
	retFn := func(att int, req *http.Request) (*http.Response, error) {
		return nil, tempErr{}
	}
	mock := &mockRoundTripper{t: t, retFn: retFn}

	tr := NewTransport(mock, RetryAll(RetryMaxRetries(1), RetryTemporaryErr()), ConstDelay(0))

	client := &http.Client{
		Transport: tr,
	}
	_, err := client.Post("http://example.com", "text/plain", iotest.TimeoutReader(strings.NewReader("hello")))
	if assert.NotNil(t, err) {
		uerr, ok := err.(*url.Error)
		require.True(t, ok)
		assert.Equal(t, iotest.ErrTimeout, uerr.Err)
	}
	assert.Equal(t, 0, mock.Calls())
}

func TestMockClientPreventRetryWithBody(t *testing.T) {
	retFn := func(att int, req *http.Request) (*http.Response, error) {
		return nil, tempErr{}
	}
	mock := &mockRoundTripper{t: t, retFn: retFn}

	tr := NewTransport(mock, RetryAll(RetryMaxRetries(1), RetryTemporaryErr()), ConstDelay(0))
	tr.PreventRetryWithBody = true

	client := &http.Client{
		Transport: tr,
	}

	_, err := client.Post("http://example.com", "text/plain", strings.NewReader("test"))
	if assert.NotNil(t, err) {
		uerr, ok := err.(*url.Error)
		require.True(t, ok)
		assert.Equal(t, tempErr{}, uerr.Err)
	}
	assert.Equal(t, 1, mock.Calls()) // did not retry
	assert.Equal(t, []string{"test"}, mock.Bodies())
}

func TestMockClientRetryWithBody(t *testing.T) {
	retFn := func(att int, req *http.Request) (*http.Response, error) {
		return nil, tempErr{}
	}
	mock := &mockRoundTripper{t: t, retFn: retFn}

	tr := NewTransport(mock, RetryAll(RetryMaxRetries(1), RetryTemporaryErr()), ConstDelay(0))

	client := &http.Client{
		Transport: tr,
	}
	_, err := client.Post("http://example.com", "text/plain", strings.NewReader("hello"))
	if assert.NotNil(t, err) {
		uerr, ok := err.(*url.Error)
		require.True(t, ok)
		assert.Equal(t, tempErr{}, uerr.Err)
	}
	assert.Equal(t, 2, mock.Calls())
	assert.Equal(t, []string{"hello", "hello"}, mock.Bodies())
}
