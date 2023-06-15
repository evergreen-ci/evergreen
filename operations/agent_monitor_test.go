package operations

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const monitorTestTimeout = 10 * time.Second

func TestAgentMonitorWithJasper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jasperPort := evergreen.DefaultJasperPort
	port := defaultMonitorPort
	manager, err := jasper.NewSynchronizedManager(false)
	require.NoError(t, err)
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", jasperPort))
	require.NoError(t, err)
	closeServer, err := remote.StartRPCService(ctx, manager, addr, nil)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, closeServer())
	}()

	for testName, testCase := range map[string]func(context.Context, *testing.T, *monitor){
		"FetchClientDownloadsFromURL": func(ctx context.Context, t *testing.T, m *monitor) {
			require.NoError(t, m.fetchClient(ctx, []string{"https://example.com"}, agentMonitorDefaultRetryOptions()))
			fileInfo, err := os.Stat(m.clientPath)
			require.NoError(t, err)
			assert.NotZero(t, fileInfo.Size())
		},
		"WaitUntilCompleteWaitsForProcessTermination": func(ctx context.Context, t *testing.T, m *monitor) {
			opts := &options.Create{Args: []string{"sleep", "1"}}
			proc, err := m.jasperClient.CreateProcess(ctx, opts)
			require.NoError(t, err)
			exitCode, err := waitUntilComplete(ctx, proc, time.Second)
			require.NoError(t, err)
			assert.True(t, proc.Complete(ctx))
			assert.Zero(t, exitCode)
		},
		"SetupLoggingCreatesExpectedLogger": func(ctx context.Context, t *testing.T, m *monitor) {
			originalSender := grip.GetSender()
			defer func() {
				// Since the agent monitor sets the global logger, reset the
				// sender at the end of the test to the original one.
				assert.NoError(t, grip.SetSender(originalSender))
			}()
			require.NoError(t, setupLogging(m))
			const msg = "hello world!"

			grip.GetSender().Send(message.ConvertToComposer(level.Info, msg))

			// Check the side effect that sending to the logger wrote a file to
			// the temporary directory containing the content.
			dir := filepath.Dir(m.logPrefix)
			files, err := os.ReadDir(filepath.Dir(m.logPrefix))
			require.NoError(t, err)
			require.Len(t, files, 1, "logging should produce exactly one output file")
			file, err := os.Open(filepath.Join(dir, files[0].Name()))
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
			}()
			content, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Contains(t, string(content), msg, "log message should appear in the file")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, monitorTestTimeout)
			defer tcancel()

			tmpDir := t.TempDir()

			m := &monitor{
				clientPath: filepath.Join(tmpDir, "evergreen"),
				distroID:   "distro",
				logOutput:  agent.LogOutputFile,
				logPrefix:  filepath.Join(tmpDir, "agent-monitor"),
				jasperPort: jasperPort,
				port:       port,
			}

			// Monitor should be able to connect without needing credentials when
			// testing.
			require.NoError(t, m.setupJasperConnection(tctx, agentMonitorDefaultRetryOptions()))
			defer func() {
				assert.NoError(t, m.jasperClient.CloseConnection())
			}()

			testCase(tctx, t, m)
		})
	}
}
