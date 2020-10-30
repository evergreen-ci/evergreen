package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/service"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/mongodb/jasper/testutil"
	"github.com/mongodb/jasper/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseDaemon(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Run("CheckPrecondition", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			preconditions []string
			expectErr     bool
		}{
			"SucceedsWithNoPreconditions": {},
			"SucceedsIfPreconditionSucceeds": {
				preconditions: []string{"echo hello world!"},
			},
			"FailsIfPreconditionFails": {
				preconditions: []string{"false"},
				expectErr:     true,
			},
			"SucceedsIfAllPreconditionsSucceed": {
				preconditions: []string{"true", "true", "echo hello world!"},
			},
			"FailsIfAnyPreconditionFails": {
				preconditions: []string{"true", "true", "false"},
				expectErr:     true,
			},
		} {
			t.Run(testName, func(t *testing.T) {
				manager, err := jasper.NewSynchronizedManager(false)
				require.NoError(t, err)
				opts := daemonOptions{
					manager:          manager,
					preconditionCmds: testCase.preconditions,
				}
				d := newBaseDaemon(opts)
				err = d.checkPreconditions(ctx)
				if testCase.expectErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
	t.Run("Setup", func(t *testing.T) {
		sctx, scancel := context.WithCancel(ctx)
		defer scancel()
		t.Run("Succeeds", func(t *testing.T) {
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)
			d := newBaseDaemon(daemonOptions{manager: manager})
			assert.NoError(t, d.setup(sctx, scancel))
		})
		t.Run("DefaultsManager", func(t *testing.T) {
			d := newBaseDaemon(daemonOptions{})
			assert.NoError(t, d.setup(sctx, scancel))
		})
		t.Run("SucceedsIfPreconditionSucceeds", func(t *testing.T) {
			d := newBaseDaemon(daemonOptions{
				preconditionCmds: []string{"true"},
			})
			assert.NoError(t, d.setup(sctx, scancel))
		})
		t.Run("FailsIfPreconditionFails", func(t *testing.T) {
			d := newBaseDaemon(daemonOptions{
				preconditionCmds: []string{"false"},
			})
			assert.Error(t, d.setup(sctx, scancel))
		})
	})
}

func TestDaemon(t *testing.T) {
	for daemonAndClientName, makeDaemonAndClient := range map[string]func(ctx context.Context, t *testing.T, manager jasper.Manager) (util.CloseFunc, remote.Manager){
		"RPCService": func(ctx context.Context, t *testing.T, _ jasper.Manager) (util.CloseFunc, remote.Manager) {
			port := testutil.GetPortNumber()
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			opts := daemonOptions{
				host:    "localhost",
				port:    port,
				manager: manager,
			}
			daemon := newRPCDaemon(opts, "")
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))

			client, err := newRemoteManager(ctx, RPCService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"RESTService": func(ctx context.Context, t *testing.T, _ jasper.Manager) (util.CloseFunc, remote.Manager) {
			port := testutil.GetPortNumber()
			manager, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			opts := daemonOptions{
				host:    "localhost",
				port:    port,
				manager: manager,
			}
			daemon := newRESTDaemon(opts)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			require.NoError(t, testutil.WaitForRESTService(ctx, fmt.Sprintf("http://localhost:%d/jasper/v1", port)))

			client, err := newRemoteManager(ctx, RESTService, "localhost", port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRESTClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (util.CloseFunc, remote.Manager) {
			restOpts := daemonOptions{
				host:    "localhost",
				port:    testutil.GetPortNumber(),
				manager: manager,
			}
			rpcOpts := daemonOptions{
				host:    "localhost",
				port:    testutil.GetPortNumber(),
				manager: manager,
			}
			daemon := newCombinedDaemon(
				newRESTDaemon(restOpts),
				newRPCDaemon(rpcOpts, ""),
			)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))
			require.NoError(t, testutil.WaitForRESTService(ctx, fmt.Sprintf("http://localhost:%d/jasper/v1", restOpts.port)))

			client, err := newRemoteManager(ctx, RESTService, "localhost", restOpts.port, "")
			require.NoError(t, err)

			return func() error { return daemon.Stop(svc) }, client
		},
		"CombinedServiceRPCClient": func(ctx context.Context, t *testing.T, manager jasper.Manager) (util.CloseFunc, remote.Manager) {
			restOpts := daemonOptions{
				host:    "localhost",
				port:    testutil.GetPortNumber(),
				manager: manager,
			}
			rpcOpts := daemonOptions{
				host:    "localhost",
				port:    testutil.GetPortNumber(),
				manager: manager,
			}
			daemon := newCombinedDaemon(
				newRESTDaemon(restOpts),
				newRPCDaemon(rpcOpts, ""),
			)
			svc, err := service.New(daemon, &service.Config{Name: "foo"})
			require.NoError(t, err)
			require.NoError(t, daemon.Start(svc))

			client, err := newRemoteManager(ctx, RPCService, "localhost", rpcOpts.port, "")
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
