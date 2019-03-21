package bond

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

const httpClientTimeout = 10 * time.Minute

var httpClientPool *sync.Pool

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
					IdleConnTimeout:     20 * time.Second,
					MaxIdleConnsPerHost: 10,
					MaxIdleConns:        50,
					Dial: (&net.Dialer{
						Timeout:   30 * time.Second,
						KeepAlive: 0,
					}).Dial,
					TLSHandshakeTimeout: 10 * time.Second,
				},
			}
		},
	}
}

func GetHTTPClient() *http.Client { return httpClientPool.Get().(*http.Client) }
func PutHTTPClient(c *http.Client) {
	c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify = false
	httpClientPool.Put(c)
}
