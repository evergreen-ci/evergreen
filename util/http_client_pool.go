package util

import (
	"crypto/tls"
	"net"
	"net/http"
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

func GetHTTPClient() *http.Client { return httpClientPool.Get().(*http.Client) }

func PutHTTPClient(c *http.Client) {
	c.Timeout = httpClientTimeout

	switch c.Transport.(type) {
	case *http.Transport:
		c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
		httpClientPool.Put(c)
	case *oauth2.Transport:
		oauthClientPool.put(c)
	case *rehttp.Transport:
		c.Transport = c.Transport.(*rehttp.Transport).RoundTripper
		PutHTTPClient(c)
	default:
		c.Transport = newConfiguredBaseTransport()
		httpClientPool.Put(c)
	}
}

func GetOAuth2HTTPClient(oauthToken string) (*http.Client, error) {
	if oauthToken == "" {
		return nil, errors.New("oauth token cannot be empty")
	}

	return oauthClientPool.getOrMake(oauthToken).Get().(*http.Client), nil
}

func GetRetryableOauth2HTTPClient(oauthToken string, fRetry rehttp.RetryFn, fDelay rehttp.DelayFn) (*http.Client, error) {
	client, err := GetOAuth2HTTPClient(oauthToken)

	if err != nil {
		return nil, errors.Wrap(err, "failed to oauth client")
	}

	client.Transport = rehttp.NewTransport(client.Transport, fRetry, fDelay)

	return client, nil
}

type HTTPRetryConfiguration struct {
	MaxDelay        time.Duration
	BaseDelay       time.Duration
	MaxRetries      int
	TemporaryErrors bool
	Methods         []string
	Statuses        []int
}

func NewDefaultHTTPRetryConf() HTTPRetryConfiguration {
	return HTTPRetryConfiguration{
		MaxRetries:      100,
		TemporaryErrors: true,
		MaxDelay:        5 * time.Second,
		BaseDelay:       50 * time.Millisecond,
		Methods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
		},
		Statuses: []int{
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusInsufficientStorage,
			http.StatusConflict,
			http.StatusRequestTimeout,
			http.StatusPreconditionFailed,
			http.StatusExpectationFailed,
		},
	}
}

func GetHTTPRetryableClient(conf HTTPRetryConfiguration) *http.Client {
	client := GetHTTPClient()

	statusRetries := []rehttp.RetryFn{}
	if len(conf.Statuses) > 0 {
		statusRetries = append(statusRetries, rehttp.RetryStatuses(conf.Statuses...))
	} else {
		conf.TemporaryErrors = true
	}

	if conf.TemporaryErrors {
		statusRetries = append(statusRetries, rehttp.RetryTemporaryErr())
	}

	retryFns := []rehttp.RetryFn{rehttp.RetryAny(statusRetries...)}

	if len(conf.Methods) > 0 {
		retryFns = append(retryFns, rehttp.RetryHTTPMethods(conf.Methods...))
	}

	if conf.MaxRetries > 0 {
		retryFns = append(retryFns, rehttp.RetryMaxRetries(conf.MaxRetries))
	}

	client.Transport = rehttp.NewTransport(client.Transport,
		rehttp.RetryAll(retryFns...),
		rehttp.ExpJitterDelay(conf.BaseDelay, conf.MaxDelay))

	return client
}

func GetDefaultHTTPRetryableClient() *http.Client {
	return GetHTTPRetryableClient(NewDefaultHTTPRetryConf())
}
