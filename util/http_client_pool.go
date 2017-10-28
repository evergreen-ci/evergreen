package util

import (
	"net/http"
	"sync"
)

var httpClientPool *sync.Pool

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} {
			return &http.Client{}
		},
	}
}

func GetHttpClient() *http.Client  { return httpClientPool.Get().(*http.Client) }
func PutHttpClient(c *http.Client) { httpClientPool.Put(c) }
