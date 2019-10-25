package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

func startRESTService(ctx context.Context, client *http.Client) (*Service, int, error) {
outerRetry:
	for {
		select {
		case <-ctx.Done():
			grip.Warning("timed out starting test service")
			return nil, -1, errors.WithStack(ctx.Err())
		default:
			synchronizedManager, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return nil, -1, errors.WithStack(err)
			}
			srv := NewManagerService(synchronizedManager)
			app := srv.App(ctx)
			app.SetPrefix("jasper")

			if err := app.SetHost("localhost"); err != nil {
				continue outerRetry
			}

			port := testutil.GetPortNumber()
			if err := app.SetPort(port); err != nil {
				continue outerRetry
			}

			go func() {
				grip.Warning(app.Run(ctx))
			}()

			timer := time.NewTimer(5 * time.Millisecond)
			defer timer.Stop()
			url := fmt.Sprintf("http://localhost:%d/jasper/v1/", port)

			trials := 0
		checkLoop:
			for {
				if trials > 10 {
					continue outerRetry
				}

				select {
				case <-ctx.Done():
					return nil, -1, errors.WithStack(ctx.Err())
				case <-timer.C:
					req, err := http.NewRequest(http.MethodGet, url, nil)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}
					rctx, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					req = req.WithContext(rctx)
					resp, err := client.Do(req)
					if err != nil {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}
					if resp.StatusCode != http.StatusOK {
						timer.Reset(5 * time.Millisecond)
						trials++
						continue checkLoop
					}

					return srv, port, nil
				}
			}
		}
	}
}
