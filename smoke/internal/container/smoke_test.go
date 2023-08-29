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

	appServerCmd := internal.StartAppServer(ctx, t, params.APIParams)
	defer func() {
		if appServerCmd != nil {
			grip.Error(errors.Wrap(appServerCmd.Signal(ctx, syscall.SIGTERM), "stopping app server after test completion"))
		}
	}()

	agentCmd := internal.StartAgent(ctx, t, params.APIParams, agent.PodMode, params.execModeID, params.execModeSecret)
	defer func() {
		if agentCmd != nil {
			grip.Error(errors.Wrap(agentCmd.Signal(ctx, syscall.SIGTERM), "stopping agent after test completion"))
		}
	}()

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	internal.WaitForEvergreen(t, params.AppServerURL, client)

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
	execModeSecret := os.Getenv("EXEC_MODE_SECRET")
	if execModeSecret == "" {
		execModeSecret = "de249183582947721fdfb2ea1796574b"
	}

	taskID := os.Getenv("TASK_ID")
	if taskID == "" {
		taskID = "evergreen_pod_bv_container_task_a71e20e60918bb97d41422e94d04822be2a22e8e_22_08_22_13_44_49"
	}

	return smokeTestParams{
		APIParams:      internal.GetAPIParamsFromEnv(t, evgHome),
		execModeID:     execModeID,
		execModeSecret: execModeSecret,
		taskID:         taskID,
	}

}
