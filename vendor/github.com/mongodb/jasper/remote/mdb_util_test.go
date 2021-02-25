package remote

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

func makeTestMDBServiceAndClient(ctx context.Context, mngr jasper.Manager) (Manager, error) {
	var (
		addr   net.Addr
		client Manager
		err    error
	)
tryPort:
	for {
		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "starting MDB service")
		default:
			addr, err = net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
			if err != nil {
				continue tryPort
			}

			srvCtx, srvCancel := context.WithCancel(ctx)
			closeService, err := StartMDBService(srvCtx, mngr, addr)
			if err != nil {
				srvCancel()
				return nil, errors.WithStack(err)
			}

			failedToConnect := make(chan struct{})
			go func() {
				defer func() {
					srvCancel()
					grip.Notice(closeService())
				}()
				select {
				case <-ctx.Done():
				case <-failedToConnect:
				}
			}()

			client, err = tryConnectToMDBService(ctx, addr)
			if err != nil {
				close(failedToConnect)
				continue
			}
			break tryPort
		}
	}

	go func() {
		<-ctx.Done()
		grip.Notice(client.CloseConnection())
	}()
	return client, nil
}

func tryConnectToMDBService(ctx context.Context, addr net.Addr) (Manager, error) {
	maxAttempts := 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		client, err := func() (Manager, error) {
			connCtx, connCancel := context.WithTimeout(ctx, time.Second)
			defer connCancel()
			client, err := waitForMDBService(connCtx, addr)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return client, nil
		}()
		if err != nil {
			continue
		}
		return client, nil
	}
	return nil, errors.Errorf("failed to connect after %d attempts", maxAttempts)
}

// waitForMDBService waits until either the MDB wire protocol service becomes
// available to serve requests or the context times out.
func waitForMDBService(ctx context.Context, addr net.Addr) (Manager, error) {
	// Block until the service comes up
	timeoutInterval := 10 * time.Millisecond
	timer := time.NewTimer(timeoutInterval)
	for {
		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "context done before connection could be established to service")
		case <-timer.C:
			client, err := NewMDBClient(ctx, addr, 0)
			if err != nil {
				timer.Reset(timeoutInterval)
				continue
			}
			// Ensure that a client can connect to the service by verifying
			// that it returns a manager ID.
			if client.ID() == "" {
				timer.Reset(timeoutInterval)
				grip.Warning(client.CloseConnection())
				continue
			}
			return client, nil
		}
	}
}
