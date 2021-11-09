package utility

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"golang.org/x/oauth2"
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

func setupOauth2HTTPClient(token string, client *http.Client) *http.Client {
	client.Transport = &oauth2.Transport{
		Base: client.Transport,
		Source: oauth2.ReuseTokenSource(nil, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)),
	}
	return client
}

// GetHTTPClient produces default HTTP client from the pool,
// constructing a new client if needed. Always pair calls to
// GetHTTPClient with defered calls to PutHTTPClient.
func GetHTTPClient() *http.Client { return httpClientPool.Get().(*http.Client) }

// PutHTTPClient returns the client to the pool, automatically
// reconfiguring the transport.
func PutHTTPClient(c *http.Client) {
	c.Timeout = httpClientTimeout

	switch transport := c.Transport.(type) {
	case *http.Transport:
		transport.TLSClientConfig.InsecureSkipVerify = false
		c.Transport = transport
	case *rehttp.Transport:
		c.Transport = transport.RoundTripper
		PutHTTPClient(c)
		return
	case *oauth2.Transport:
		c.Transport = transport.Base
		PutHTTPClient(c)
		return
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
	Errors          []error
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

	if len(conf.Errors) > 0 {
		statusRetries = append(statusRetries, rehttp.RetryIsErr(func(err error) bool {
			for _, errToCheck := range conf.Errors {
				if err == errToCheck {
					return true
				}
			}
			return false
		}))
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

// HTTPRetryFunction makes it possible to write customizable retry
// logic. Returning true if the request should be retried again and
// false otherwise.
type HTTPRetryFunction func(index int, req *http.Request, resp *http.Response, err error) bool

// HTTPDelayFunction makes it possible to write customizable retry
// backoff logic, by allowing you to evaluate the previous request and
// response and return the duration to wait before the next request.
type HTTPDelayFunction func(index int, req *http.Request, resp *http.Response, err error) time.Duration

func makeRetryFn(in HTTPRetryFunction) rehttp.RetryFn {
	return func(attempt rehttp.Attempt) bool {
		return in(attempt.Index, attempt.Request, attempt.Response, attempt.Error)
	}
}

func makeDelayFn(in HTTPDelayFunction) rehttp.DelayFn {
	return func(attempt rehttp.Attempt) time.Duration {
		return in(attempt.Index, attempt.Request, attempt.Response, attempt.Error)
	}
}

// GetCustomHTTPRetryableClient allows you to generate an HTTP client
// that automatically retries failed request based on the provided
// custom logic.
func GetCustomHTTPRetryableClient(retry HTTPRetryFunction, delay HTTPDelayFunction) *http.Client {
	client := GetHTTPClient()
	client.Transport = rehttp.NewTransport(client.Transport, makeRetryFn(retry), makeDelayFn(delay))
	return client
}

// GetOAuth2HTTPClient produces an HTTP client that will supply OAuth2
// credentials with all requests. There is no validation of the
// token, and you should always call PutHTTPClient to return the
// client to the pool when you're done with it.
func GetOAuth2HTTPClient(oauthToken string) *http.Client {
	return setupOauth2HTTPClient(oauthToken, GetHTTPClient())
}

// GetOauth2DefaultHTTPRetryableClient constructs an HTTP client that
// supplies OAuth2 credentials with all requests, retrying failed
// requests automatically according to the default retryable
// options. There is no validation of the token, and you should always
// call PutHTTPClient to return the client to the pool when you're
// done with it.
func GetOauth2DefaultHTTPRetryableClient(oauthToken string) *http.Client {
	return setupOauth2HTTPClient(oauthToken, GetDefaultHTTPRetryableClient())
}

// GetOauth2HTTPRetryableClient constructs an HTTP client that
// supplies OAuth2 credentials with all requests, retrying failed
// requests automatically according to the configuration
// provided. There is no validation of the token, and you should
// always call PutHTTPClient to return the client to the pool when
// you're done with it.
func GetOauth2HTTPRetryableClient(oauthToken string, conf HTTPRetryConfiguration) *http.Client {
	return setupOauth2HTTPClient(oauthToken, GetHTTPRetryableClient(conf))
}

// GetOauth2HTTPRetryableClient constructs an HTTP client that
// supplies OAuth2 credentials with all requests, retrying failed
// requests automatically according to definitions of the provided
// functions. There is no validation of the token, and you should
// always call PutHTTPClient to return the client to the pool when
// you're done with it.
func GetOauth2CustomHTTPRetryableClient(token string, retry HTTPRetryFunction, delay HTTPDelayFunction) *http.Client {
	return setupOauth2HTTPClient(token, GetCustomHTTPRetryableClient(retry, delay))
}

// TemporayError defines an interface for use in retryable HTTP
// clients to identify certain errors as Temporary.
type TemporaryError interface {
	error
	Temporary() bool
}

// IsTemporaryError returns true if the error object is also a
// temporary error.
func IsTemporaryError(err error) bool {
	if terr, ok := err.(TemporaryError); ok {
		return terr.Temporary()
	}
	return false
}
