package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemon(t *testing.T) {
	for daemonAndClientName, makeDaemonAndClient := range map[string]func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient){
		"RPCService": func(ctx context.Context, t *testing.T, _ jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			port := testutil.GetPortNumber()
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			daemon := newRPCDaemon("localhost", port, manager, "", nil)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))

			client, err := newRemoteClient(ctx, RPCService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"RESTService": func(ctx context.Context, t *testing.T, _ jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			port := testutil.GetPortNumber()
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			daemon := newRESTDaemon("localhost", port, manager, nil)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			require.NoError(t, testutil.WaitForRESTService(ctx, fmt.Sprintf("http://localhost:%d/jasper/v1", port)))

			client, err := newRemoteClient(ctx, RESTService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRESTClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			restPort := testutil.GetPortNumber()
			daemon := newCombinedDaemon(
				newRESTDaemon("localhost", restPort, manager, nil),
				newRPCDaemon("localhost", testutil.GetPortNumber(), manager, "", nil),
			)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			require.NoError(t, testutil.WaitForRESTService(ctx, fmt.Sprintf("http://localhost:%d/jasper/v1", restPort)))

			client, err := newRemoteClient(ctx, RESTService, "localhost", restPort, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRPCClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			rpcPort := testutil.GetPortNumber()
			daemon := newCombinedDaemon(
				newRESTDaemon("localhost", testutil.GetPortNumber(), manager, nil),
				newRPCDaemon("localhost", rpcPort, manager, "", nil),
			)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))

			client, err := newRemoteClient(ctx, RPCService, "localhost", rpcPort, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
	} {
		t.Run(daemonAndClientName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
			defer cancel()

			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			closeDaemon, client := makeDaemonAndClient(ctx, t, manager)
			defer func() {
				assert.NoError(t, closeDaemon())
			}()

			opts := &options.Create{
				Args: []string{"echo", "hello", "world"},
			}
			proc, err := client.CreateProcess(ctx, opts)
			require.NoError(t, err)

			exitCode, err := proc.Wait(ctx)
			require.NoError(t, err)
			assert.Zero(t, exitCode)
		})
	}
}
