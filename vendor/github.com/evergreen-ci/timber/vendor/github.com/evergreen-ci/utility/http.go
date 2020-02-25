package utility

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/PuerkitoBio/rehttp"
)

const httpClientTimeout = 5 * time.Minute

var httpClientPool *sync.Pool

func init() {
	initHTTPPool()
}

func initHTTPPool() {
	httpClientPool = &sync.Pool{
		New: func() interface{} { return newBaseConfiguredHttpClient() },
	}
}

func newBaseConfiguredHttpClient() *http.Client {
	return &http.Client{
		Timeout:   httpClientTimeout,
		Transport: newConfiguredBaseTransport(),
	}
}

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

// GetHTTPClient produces default HTTP client from the pool,
// constructing a new client if needed. Always pair calls to
// GetHTTPClient with defered calls to PutHTTPClient.
func GetHTTPClient() *http.Client { return httpClientPool.Get().(*http.Client) }

func PutHTTPClient(c *http.Client) {
	c.Timeout = httpClientTimeout

	switch transport := c.Transport.(type) {
	case *http.Transport:
		transport.TLSClientConfig.InsecureSkipVerify = false
		c.Transport = transport
	case *rehttp.Transport:
		c.Transport = transport.RoundTripper
		PutHTTPClient(c)
	default:
		c.Transport = newConfiguredBaseTransport()
	}

	httpClientPool.Put(c)
}

// HTTPRetryConfiguration makes it possible to configure the retry
// semantics for retryable clients. In most cases, construct this
// object using the NewDefaultHttpRetryConf, which provides reasonable
// defaults.
type HTTPRetryConfiguration struct {
	MaxDelay        time.Duration
	BaseDelay       time.Duration
	MaxRetries      int
	TemporaryErrors bool
	Methods         []string
	Statuses        []int
}

// NewDefaultHTTPRetryConf constructs a HTTPRetryConfiguration object
// with reasonable defaults.
func NewDefaultHTTPRetryConf() HTTPRetryConfiguration {
	return HTTPRetryConfiguration{
		MaxRetries:      50,
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

// GetHTTPRetryableClient produces an HTTP client that automatically
// retries failed requests according to the configured
// parameters. Couple calls to GetHTTPRetryableClient, with defered
// calls to PutHTTPClient.
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

// GetDefaultHTTPRetryableClient provides a retryable client with
// the default settings. Couple calls to GetHTTPRetryableClient, with defered
// calls to PutHTTPClient.
func GetDefaultHTTPRetryableClient() *http.Client {
	return GetHTTPRetryableClient(NewDefaultHTTPRetryConf())
}
