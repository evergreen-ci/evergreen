package utility

import (
	"net/http"
	"testing"

	"github.com/PuerkitoBio/rehttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
