package util

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var httpClientPool *sync.Pool

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} {
			return &http.Client{
				Transport: &http.Transport{
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

func GetHttpClient() *http.Client  { return httpClientPool.Get().(*http.Client) }
func PutHttpClient(c *http.Client) { httpClientPool.Put(c) }
