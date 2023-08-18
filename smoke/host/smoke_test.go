package host

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestSmokeHostTask runs the smoke test for a host task.
func TestSmokeHostTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := GetSmokeTestParamsFromEnv(t)
	grip.Info(message.Fields{
		"message": "got smoke test parameters",
		"params":  fmt.Sprintf("%#v", params),
	})

	appServerCmd, err := internal.StartAppServer(ctx, t, params.APIParams)
	require.NoError(t, err)
	defer func() {
		if appServerCmd != nil && appServerCmd.Process != nil {
			grip.Error(errors.Wrap(appServerCmd.Process.Signal(syscall.SIGTERM), "stopping app server after test completion"))
		}
	}()

	agentCmd, err := internal.StartAgent(ctx, t, params.APIParams, agent.HostMode, params.ExecModeID, params.ExecModeSecret)
	require.NoError(t, err)
	defer func() {
		if agentCmd != nil && agentCmd.Process != nil {
			grip.Error(errors.Wrap(agentCmd.Process.Signal(syscall.SIGTERM), "stopping agent after test completion"))
		}
	}()

	RunHostTaskPatchTest(ctx, t, params)
}
