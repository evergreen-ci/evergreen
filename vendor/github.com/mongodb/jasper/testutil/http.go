package testutil

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var httpClientPool *sync.Pool

func init() {
	httpClientPool = &sync.Pool{
		New: func() interface{} {
			return &http.Client{}
		},
	}
}

// GetHTTPClient gets an HTTP client from the client pool.
func GetHTTPClient() *http.Client {
	return httpClientPool.Get().(*http.Client)
}

// PutHTTPClient returns the given HTTP client back to the pool.
func PutHTTPClient(client *http.Client) {
	httpClientPool.Put(client)
}

// WaitForRESTService waits until either the REST service becomes available to
// serve requests or the context is done.
func WaitForRESTService(ctx context.Context, url string) error {
	client := GetHTTPClient()
	defer PutHTTPClient(client)

	// Block until the service comes up
	timeoutInterval := 10 * time.Millisecond
	timer := time.NewTimer(timeoutInterval)
	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case <-timer.C:
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			req = req.WithContext(ctx)
			resp, err := client.Do(req)
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				timer.Reset(timeoutInterval)
				continue
			}
			return nil
		}
	}
}

// WaitForWireService waits unti either the wire service becomes available to
// serve requests or the context times ou t.
func WaitForWireService(ctx context.Context, addr net.Addr) error { //nolint: interfacer
	// Block until the service comes up
	timeoutInterval := 10 * time.Millisecond
	timer := time.NewTimer(timeoutInterval)
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context errored before connection could be established to service")
		case <-timer.C:
			conn, err := net.Dial("tcp", addr.String())
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			grip.Warning(conn.Close())
			return nil
		}
	}
}
