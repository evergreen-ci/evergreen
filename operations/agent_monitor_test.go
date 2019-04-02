package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// kim: TODO: integration tests with Jasper RPC service.
// 1. Set up Jasper.
// 2. Test fetch client using download.
// 3. Test

// startLocalJasperService starts the Jasper service on localhost.
func startLocalJasperService(ctx context.Context, manager jasper.Manager, port int) error {
	addr := fmt.Sprintf("localhost:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.WithStack(err)
	}

	service := grpc.NewServer()

	if err = rpc.AttachService(manager, service); err != nil {
		return errors.WithStack(err)
	}

	go service.Serve(lis)

	go func() {
		<-ctx.Done()
		service.Stop()
	}()

	return nil
}

const monitorTestTimeout = 10 * time.Second

func TestMonitorWithJasper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := defaultMonitorPort
	jasperPort := defaultJasperPort
	manager, err := jasper.NewLocalManager(false)
	require.NoError(t, err)
	require.NoError(t, startLocalJasperService(ctx, manager, jasperPort))

	for testName, testCase := range map[string]func(context.Context, *testing.T, *monitor){
		"FetchClientDownloadsFromURL": func(ctx context.Context, t *testing.T, mon *monitor) {
			require.NoError(t, mon.fetchClient(ctx, time.Minute))
			fileInfo, err := os.Stat(mon.clientPath)
			require.NoError(t, err)
			assert.NotZero(t, fileInfo.Size())
		},
		"WaitUntilCompleteWaitsForProcessTermination": func(ctx context.Context, t *testing.T, mon *monitor) {
			opts := &jasper.CreateOptions{Args: []string{"sleep", "1"}}
			proc, err := mon.jasperClient.CreateProcess(ctx, opts)
			require.NoError(t, err)
			exitCode, err := waitUntilComplete(ctx, proc, time.Second)
			require.NoError(t, err)
			assert.True(t, proc.Complete(ctx))
			assert.Zero(t, exitCode)
		},
		// "": func(context.Context, *testing.T, *monitor)
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, monitorTestTimeout)
			defer tcancel()

			tmpDir, err := ioutil.TempDir("", "monitor")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			m := &monitor{
				clientURL:  "https://www.example.com",
				clientPath: filepath.Join(tmpDir, "evergreen"),
				jasperPort: jasperPort,
				port:       port,
			}

			// Monitor should be able to connect without needing credentials when testing.
			conn, err := m.setupJasperConnection(context.Background(), time.Minute)
			require.NoError(t, err)
			defer conn.Close()

			testCase(tctx, t, m)
		})
	}
}
