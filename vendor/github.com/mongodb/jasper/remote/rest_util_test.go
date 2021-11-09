package remote

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

func makeRESTServiceAndClient(ctx context.Context, mngr jasper.Manager, httpClient *http.Client) (*Service, Manager, error) {
tryStartService:
	for {
		select {
		case <-ctx.Done():
			return nil, nil, errors.WithStack(ctx.Err())
		default:
			srv := NewRESTService(mngr)
			app := srv.App(ctx)
			app.SetPrefix("jasper")

			if err := app.SetHost("localhost"); err != nil {
				continue tryStartService
			}

			port := testutil.GetPortNumber()
			if err := app.SetPort(port); err != nil {
				continue tryStartService
			}

			srvCtx, srvCancel := context.WithCancel(ctx)
			go func() {
				grip.Warning(app.Run(srvCtx))
			}()

			failedToConnect := make(chan struct{})
			go func() {
				defer srvCancel()
				select {
				case <-ctx.Done():
				case <-failedToConnect:
				}
			}()

			addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
			if err != nil {
				continue
			}
			url := fmt.Sprintf("http://%s/jasper/v1/", addr)
			if err := tryConnectToRESTService(ctx, url, httpClient); err != nil {
				close(failedToConnect)
				continue
			}
			return srv, newTestRESTClient(ctx, addr, httpClient), nil
		}
	}
}

func tryConnectToRESTService(ctx context.Context, url string, httpClient *http.Client) error {
	maxAttempts := 10
	for attempt := 0; attempt < 10; attempt++ {
		err := func() error {
			connCtx, connCancel := context.WithTimeout(ctx, time.Second)
			defer connCancel()
			if err := testutil.WaitForHTTPService(connCtx, url, httpClient); err != nil {
				return errors.WithStack(err)
			}
			return nil
		}()
		if err != nil {
			continue
		}
		return nil
	}
	return errors.Errorf("failed to connect after %d attempts", maxAttempts)
}

// newTestRESTClient establishes a client for testing purposes that closes when
// the context is done.
func newTestRESTClient(ctx context.Context, addr net.Addr, httpClient *http.Client) Manager {
	client := NewRESTClientWithExistingClient(addr, httpClient)

	go func() {
		<-ctx.Done()
		grip.Notice(client.CloseConnection())
	}()

	return client
}
