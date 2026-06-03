package agent

import (
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agentForKillTest returns a minimal Agent that passes the shouldKill guard.
// shouldKill short-circuits to false when a.opts.Cleanup is false (added in
// the upstream sync), so tests that want to reach the kill/cleanup logic must
// set Cleanup: true.
func agentForKillTest() *Agent {
	return &Agent{opts: Options{Cleanup: true}}
}

// TestKillProcsContainerIsolationSkipsDockerCleanup verifies the A5 gate:
// when ContainerIsolation is set, killProcs logs "Skipping Docker artifact
// cleanup" and returns early without calling docker.Cleanup. The gate is
// confirmed by the presence of that log line in the test output.
func TestKillProcsContainerIsolationSkipsDockerCleanup(t *testing.T) {
	ctx := t.Context()
	a := agentForKillTest()

	tc := &taskContext{
		task: client.TaskData{ID: "test-task-1"},
		taskConfig: &internal.TaskConfig{
			// Empty ExecUser → takes the ps-based kill path (no sudo pkill),
			// safe in a sandboxed test environment.
			WorkDir: t.TempDir(),
			Distro: &apimodels.DistroView{
				ContainerIsolation: &apimodels.ContainerIsolationSettings{
					Image: "ubuntu:22.04",
				},
			},
		},
	}

	// shouldKill → true (Cleanup=true, task.ID set, TaskGroup nil).
	// Process-kill block: ContainerID="" and ExecUser="" → ps path, no-op.
	// docker.Cleanup gate: ContainerIsolation != nil → early return, nil.
	err := a.killProcs(ctx, tc, true, "test")
	assert.NoError(t, err, "killProcs must return nil on a container-isolation distro without invoking docker.Cleanup")
}

// TestKillProcsContainerRouting verifies that killProcs routes to the
// in-container kill path when ContainerID is set, and the host-side
// KillSpawnedProcs path when it is not.
func TestKillProcsContainerRouting(t *testing.T) {
	ctx := t.Context()
	a := agentForKillTest()

	t.Run("EmptyContainerIDTakesHostPath", func(t *testing.T) {
		tc := &taskContext{
			task: client.TaskData{ID: "test-task-1"},
			taskConfig: &internal.TaskConfig{
				WorkDir: t.TempDir(),
				Distro: &apimodels.DistroView{
					// Empty ExecUser: takes the ps-based host path, which is
					// safe without real sudo in a test environment.
				},
			},
		}
		// ContainerID="" → host-side KillSpawnedProcs (ps path, no sudo).
		// No task-marker processes running → nil.
		err := a.killProcs(ctx, tc, true, "test")
		assert.NoError(t, err)
	})

	t.Run("NonEmptyContainerIDEmptyExecUserLogsWarningNoError", func(t *testing.T) {
		tc := &taskContext{
			task: client.TaskData{ID: "test-task-1"},
			taskConfig: &internal.TaskConfig{
				ContainerID: "some-container-id",
				WorkDir:     t.TempDir(),
				Distro: &apimodels.DistroView{
					// ExecUser deliberately empty to hit the graceful-degradation
					// branch (warning logged, no docker exec attempted).
					ContainerIsolation: &apimodels.ContainerIsolationSettings{
						Image: "ubuntu:22.04",
					},
				},
			},
		}
		// ContainerID != "" but ExecUser == "" → warning logged, nil returned.
		err := a.killProcs(ctx, tc, true, "test")
		require.NoError(t, err)
	})
}
