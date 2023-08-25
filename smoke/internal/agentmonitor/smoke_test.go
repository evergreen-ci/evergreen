package agentmonitor

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/evergreen-ci/evergreen/smoke/internal/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestSmokeAgentMonitor runs the smoke test for the agent monitor. This is
// mostly same set of checks as the host smoke test, but it runs the agent using
// the agent monitor rather than directly starting the agent.
func TestSmokeAgentMonitor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := getSmokeTestParamsFromEnv(t)
	grip.Info(message.Fields{
		"message": "got smoke test parameters",
		"params":  fmt.Sprintf("%#v", params),
	})

	appServerCmd := internal.StartAppServer(ctx, t, params.APIParams)
	defer func() {
		if appServerCmd != nil {
			grip.Error(errors.Wrap(appServerCmd.Signal(ctx, syscall.SIGTERM), "stopping app server after test completion"))
		}
	}()

	agentMonitorCmd := startAgentMonitor(ctx, t, params)
	defer func() {
		if agentMonitorCmd != nil {
			grip.Error(errors.Wrap(agentMonitorCmd.Signal(ctx, syscall.SIGTERM), "stopping agent monitor after test completion"))
		}
	}()

	host.RunHostTaskPatchTest(ctx, t, params.SmokeTestParams)
}

type smokeTestParams struct {
	host.SmokeTestParams
	distroID string
}

// getSmokeTestParamsFromEnv gets the necessary parameters for the agent monitor
// smoke test. It sets defaults where possible. Note that the default data
// depends on the setup test data for the smoke test.
func getSmokeTestParamsFromEnv(t *testing.T) smokeTestParams {
	distroID := os.Getenv("DISTRO_ID")
	if distroID == "" {
		distroID = "localhost"
	}

	return smokeTestParams{
		SmokeTestParams: host.GetSmokeTestParamsFromEnv(t),
		distroID:        distroID,
	}
}

// startAgentMonitor starts the smoke test agent monitor.
func startAgentMonitor(ctx context.Context, t *testing.T, params smokeTestParams) jasper.Process {
	grip.Info("Starting smoke test agent monitor.")

	agentCmd, err := internal.SmokeRunBinary(ctx,
		"smoke-agent-monitor",
		params.EVGHome,
		params.CLIPath,
		"service",
		"deploy",
		"start-evergreen",
		"--monitor",
		fmt.Sprintf("--exec_mode_id=%s", params.ExecModeID),
		fmt.Sprintf("--exec_mode_secret=%s", params.ExecModeSecret),
		fmt.Sprintf("--distro=%s", params.distroID),
		fmt.Sprintf("--api_server=%s", params.AppServerURL),
		fmt.Sprintf("--binary=%s", params.CLIPath),
	)
	require.NoError(t, err, "should have started Evergreen smoke test agent monitor")

	grip.Info("Successfully started smoke test agent monitor.")

	return agentCmd
}
