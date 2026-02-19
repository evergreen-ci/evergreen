package container

import (
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/stretchr/testify/require"
)

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
