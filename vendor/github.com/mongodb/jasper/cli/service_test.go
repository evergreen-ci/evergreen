package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/kardianos/service"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemon(t *testing.T) {
	for daemonAndClientName, makeDaemonAndClient := range map[string]func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient){
		"RPCService": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			port := getNextPort()
			manager, err := jasper.NewLocalManager(false)
			require.NoError(t, err)

			daemon := newRPCDaemon("localhost", port, manager, "")
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))

			client, err := newRemoteClient(ctx, RPCService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"RESTService": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			port := getNextPort()
			manager, err := jasper.NewLocalManager(false)
			require.NoError(t, err)

			daemon := newRESTDaemon("localhost", port, manager)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			waitForRESTService(ctx, t, fmt.Sprintf("http://localhost:%d/jasper/v1", port))

			client, err := newRemoteClient(ctx, RESTService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRESTClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			restPort := getNextPort()
			daemon := newCombinedDaemon(
				newRESTDaemon("localhost", restPort, manager),
				newRPCDaemon("localhost", getNextPort(), manager, ""),
			)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			waitForRESTService(ctx, t, fmt.Sprintf("http://localhost:%d/jasper/v1", restPort))

			client, err := newRemoteClient(ctx, RESTService, "localhost", restPort, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRPCClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (jasper.CloseFunc, jasper.RemoteClient) {
			rpcPort := getNextPort()
			daemon := newCombinedDaemon(
				newRESTDaemon("localhost", getNextPort(), manager),
				newRPCDaemon("localhost", rpcPort, manager, ""),
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
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()

			manager, err := jasper.NewLocalManager(false)
			require.NoError(t, err)
			closeDaemon, client := makeDaemonAndClient(ctx, t, manager)
			defer func() {
				assert.NoError(t, closeDaemon())
			}()

			opts := &jasper.CreateOptions{
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
