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

	appServerCmd := internal.StartAppServer(ctx, t, params.APIParams)
	defer func() {
		if appServerCmd != nil {
			grip.Error(errors.Wrap(appServerCmd.Signal(ctx, syscall.SIGTERM), "stopping app server after test completion"))
		}
	}()

	agentCmd := internal.StartAgent(ctx, t, params.APIParams, agent.HostMode, params.ExecModeID, params.ExecModeSecret)
	defer func() {
		if agentCmd != nil {
			grip.Error(errors.Wrap(agentCmd.Signal(ctx, syscall.SIGTERM), "stopping agent after test completion"))
		}
	}()

	RunHostTaskPatchTest(ctx, t, params)
}
