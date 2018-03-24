package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

type oauthPool struct {
	mu   sync.RWMutex
	pool map[string]*sync.Pool
}

func (p *oauthPool) getOrMake(oauthToken string) *sync.Pool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pool[oauthToken] == nil {
		p.pool[oauthToken] = &sync.Pool{
			New: func() interface{} {
				client := newBaseConfiguredHttpClient()
				client.Transport = &oauth2.Transport{
					Base: client.Transport,
					Source: oauth2.ReuseTokenSource(nil, oauth2.StaticTokenSource(
						&oauth2.Token{AccessToken: oauthToken},
					)),
				}
				return client
			},
		}
	}

	return p.pool[oauthToken]
}

func (p *oauthPool) put(c *http.Client) {
	tokenSource, err := c.Transport.(*oauth2.Transport).Source.Token()
	if err != nil {
		c.Transport = newConfiguredBaseTransport()
		httpClientPool.Put(c)
		return
	}
	token := tokenSource.AccessToken

	p.mu.RLock()
	defer p.mu.RUnlock()

	p.pool[token].Put(c)
}

const httpClientTimeout = 5 * time.Minute

var httpClientPool *sync.Pool
var oauthClientPool *oauthPool

func newConfiguredBaseTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig:     &tls.Config{},
		Proxy:               http.ProxyFromEnvironment,
		DisableCompression:  false,
		DisableKeepAlives:   true,
		IdleConnTimeout:     20 * time.Second,
		MaxIdleConnsPerHost: 10,
		MaxIdleConns:        50,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 0,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	}

}

func newBaseConfiguredHttpClient() *http.Client {
	return &http.Client{
		Timeout:   httpClientTimeout,
		Transport: newConfiguredBaseTransport(),
	}
}

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} { return newBaseConfiguredHttpClient() },
	}

	oauthClientPool = &oauthPool{
		pool: map[string]*sync.Pool{},
	}
}

func GetHttpClient() *http.Client { return httpClientPool.Get().(*http.Client) }

func PutHttpClient(c *http.Client) {
	c.Timeout = httpClientTimeout

	switch c.Transport.(type) {
	case *http.Transport:
		c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
		httpClientPool.Put(c)
	case *oauth2.Transport:
		oauthClientPool.put(c)
	case *rehttp.Transport:
		c.Transport = c.Transport.(*rehttp.Transport).RoundTripper
		PutHttpClient(c)
	default:
		c.Transport = newConfiguredBaseTransport()
		httpClientPool.Put(c)
	}
}

func GetOAuth2HttpClient(oauthToken string) (*http.Client, error) {
	if oauthToken == "" {
		return nil, errors.New("oauth token cannot be empty")
	}

	splitToken := strings.Split(oauthToken, " ")
	if len(splitToken) != 2 || splitToken[0] != "token" {
		return nil, errors.New("token format was invalid, expected 'token [token]'")
	}
	oauthToken = splitToken[1]

	return oauthClientPool.getOrMake(oauthToken).Get().(*http.Client), nil
}

func GetRetryableHttpClientForOauth2(oauthToken string, fRetry rehttp.RetryFn, fDelay rehttp.DelayFn) (*http.Client, error) {
	client, err := GetOAuth2HttpClient(oauthToken)

	if err != nil {
		return nil, errors.Wrap(err, "failed to oauth client")
	}

	client.Transport = rehttp.NewTransport(client.Transport, fRetry, fDelay)

	return client, nil
}
