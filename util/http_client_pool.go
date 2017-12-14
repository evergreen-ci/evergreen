package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

var httpClientPool *sync.Pool

const httpClientTimeout = 5 * time.Minute

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} {
			return &http.Client{
				Timeout: httpClientTimeout,
				Transport: &http.Transport{
					TLSClientConfig:     &tls.Config{},
					Proxy:               http.ProxyFromEnvironment,
					DisableCompression:  false,
					DisableKeepAlives:   true,
					IdleConnTimeout:     time.Minute,
					MaxIdleConnsPerHost: 10,
					MaxIdleConns:        100,
					Dial: (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 30 * time.Second,
					}).Dial,
					TLSHandshakeTimeout: 10 * time.Second,
				},
			}
		},
	}
}

func GetHttpClient() *http.Client { return httpClientPool.Get().(*http.Client) }
func GetHttpClientForOauth2(oauthToken string) (*http.Client, error) {
	if oauthToken != "" {
		splitToken := strings.Split(oauthToken, " ")
		if len(splitToken) != 2 {
			return nil, errors.New("token format was invalid, expected 'token [token]' or empty token")
		}
		oauthToken = splitToken[1]
	}
	client := httpClientPool.Get().(*http.Client)
	client.Transport = &oauth2.Transport{
		Base: client.Transport,
		Source: oauth2.ReuseTokenSource(nil, oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: oauthToken},
		)),
	}

	return client, nil

}

func PutHttpClient(c *http.Client) {
	if oauthTransport, ok := c.Transport.(*oauth2.Transport); ok {
		c.Transport = oauthTransport.Base
	}
	c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
	c.Timeout = httpClientTimeout
	httpClientPool.Put(c)
}
