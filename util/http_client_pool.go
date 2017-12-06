package util

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
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
func PutHttpClient(c *http.Client) {
	c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
	c.Timeout = httpClientTimeout
	httpClientPool.Put(c)
}
