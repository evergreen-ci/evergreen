package container

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestSmokeContainerTask runs the smoke test for a container task.
// TODO (PM-2617) Add task groups to the container task smoke test once they are
// supported
// TODO (EVG-17658): this test is highly fragile and has a chance of breaking if
// you change any of the container task YAML setup (e.g. change any
// container-related configuration in project.yml). If you change the container
// task setup, you will likely also have to change the smoke testdata. EVG-17658
// should address the issues with the smoke test.
func TestSmokeContainerTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := getSmokeTestParamsFromEnv(t)
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

	agentCmd, err := internal.StartAgent(ctx, t, params.APIParams, agent.PodMode, params.execModeID, params.execModeSecret)
	require.NoError(t, err)
	defer func() {
		if agentCmd != nil && agentCmd.Process != nil {
			grip.Error(errors.Wrap(agentCmd.Process.Signal(syscall.SIGTERM), "stopping agent after test completion"))
		}
	}()

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	internal.CheckTaskStatusAndLogs(ctx, t, params.APIParams, client, agent.PodMode, []string{params.taskID})
}

type smokeTestParams struct {
	internal.APIParams
	execModeID     string
	execModeSecret string
	taskID         string
}

// getSmokeTestParamsFromEnv gets the necessary parameters for the container
// smoke test. It sets defaults where possible. Note that the default data
// depends on the setup test data for the smoke test.
func getSmokeTestParamsFromEnv(t *testing.T) smokeTestParams {
	evgHome := evergreen.FindEvergreenHome()
	require.NotZero(t, evgHome, "EVGHOME must be set for smoke test")

	execModeID := os.Getenv("EXEC_MODE_ID")
	if execModeID == "" {
		execModeID = "localhost"
	}

	taskID := os.Getenv("TASK_ID")
	if taskID == "" {
		taskID = "evergreen_pod_bv_container_task_a71e20e60918bb97d41422e94d04822be2a22e8e_22_08_22_13_44_49"
	}

	return smokeTestParams{
		APIParams:  internal.GetAPIParamsFromEnv(t, evgHome),
		execModeID: execModeID,
		taskID:     taskID,
	}

}
