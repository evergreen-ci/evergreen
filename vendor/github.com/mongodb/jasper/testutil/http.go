package testutil

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// WaitForHTTPService waits until either the HTTP service becomes available to
// serve requests to the given URL or the context is done.
func WaitForHTTPService(ctx context.Context, url string, httpClient *http.Client) error {
	backoff := 10 * time.Millisecond
	timer := time.NewTimer(backoff)
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "context done before connection could be established to service")
		case <-timer.C:
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				timer.Reset(backoff)
				continue
			}
			req = req.WithContext(ctx)
			resp, err := httpClient.Do(req)
			if err != nil {
				timer.Reset(backoff)
				continue
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				timer.Reset(backoff)
				continue
			}
			return nil
		}
	}
}
