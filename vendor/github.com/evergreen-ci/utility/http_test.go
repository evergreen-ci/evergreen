package utility

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/rehttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestPooledHTTPClient(t *testing.T) {
	t.Run("Initialized", func(t *testing.T) {
		require.NotNil(t, httpClientPool)
		cl := GetHTTPClient()
		require.NotNil(t, cl)
		require.NotPanics(t, func() { PutHTTPClient(cl) })
	})
	t.Run("ResettingSkipCert", func(t *testing.T) {
		cl := GetHTTPClient()
		cl.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
		PutHTTPClient(cl)
		cl2 := GetHTTPClient()
		assert.Equal(t, cl, cl2)
		assert.False(t, cl.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
		assert.False(t, cl2.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	})
	t.Run("RehttpPool", func(t *testing.T) {
		initHTTPPool()
		cl := GetHTTPClient()
		clt := cl.Transport
		PutHTTPClient(cl)
		rcl := GetDefaultHTTPRetryableClient()
		assert.Equal(t, cl, rcl)
		assert.NotEqual(t, clt, rcl.Transport)
		assert.Equal(t, clt, rcl.Transport.(*rehttp.Transport).RoundTripper)
	})
}

type mockTransport struct {
	count         int
	expectedToken string
	status        int
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.count++

	resp := http.Response{
		StatusCode: t.status,
		Header:     http.Header{},
		Body:       ioutil.NopCloser(strings.NewReader("hi")),
	}

	token := req.Header.Get("Authorization")
	split := strings.Split(token, " ")
	if len(split) != 2 || split[0] != "Bearer" || split[1] != t.expectedToken {
		resp.StatusCode = http.StatusForbidden
	}
	return &resp, nil
}

func TestRetryableOauthClient(t *testing.T) {
	t.Run("Passing", func(t *testing.T) {
		c := GetOauth2DefaultHTTPRetryableClient("hi")
		defer PutHTTPClient(c)

		transport := &mockTransport{expectedToken: "hi", status: http.StatusOK}
		oldTransport := c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper
		defer func() {
			c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper = oldTransport
		}()
		c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper = transport

		resp, err := c.Get("https://example.com")
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 1, transport.count)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
	t.Run("Failed", func(t *testing.T) {
		conf := NewDefaultHTTPRetryConf()
		conf.MaxRetries = 5
		c := GetOauth2HTTPRetryableClient("hi", conf)
		defer PutHTTPClient(c)

		transport := &mockTransport{expectedToken: "hi", status: http.StatusInternalServerError}
		oldTransport := c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper
		defer func() {
			c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper = oldTransport
		}()
		c.Transport.(*oauth2.Transport).Base.(*rehttp.Transport).RoundTripper = transport

		resp, err := c.Get("https://example.com")
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 6, transport.count)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}
