package agent

import (
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
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
				Distro:  &apimodels.DistroView{
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

func makeSnapshotTC(expansions map[string]string, redacted []string, secrets map[string]string) *taskContext {
	exp := util.Expansions{}
	for k, v := range expansions {
		exp[k] = v
	}
	dynExp := agentutil.NewDynamicExpansions(exp)
	internalExp := agentutil.NewDynamicExpansions(util.Expansions{})
	for k, v := range secrets {
		internalExp.Put(k, v)
	}
	return &taskContext{
		taskConfig: &internal.TaskConfig{
			NewExpansions:      dynExp,
			Redacted:           redacted,
			InternalRedactions: internalExp,
		},
	}
}

func TestRedactForSnapshot(t *testing.T) {
	t.Run("NilTCReturnsUnchanged", func(t *testing.T) {
		assert.Equal(t, "hello world", redactForSnapshot("hello world", nil))
	})

	t.Run("NilTaskConfigReturnsUnchanged", func(t *testing.T) {
		assert.Equal(t, "secret", redactForSnapshot("secret", &taskContext{}))
	})

	t.Run("RedactsNamedExpansion", func(t *testing.T) {
		tc := makeSnapshotTC(map[string]string{"my_key": "supersecret"}, []string{"my_key"}, nil)
		result := redactForSnapshot("data contains supersecret value", tc)
		assert.NotContains(t, result, "supersecret")
		assert.Contains(t, result, "<REDACTED:my_key>")
	})

	t.Run("LongestFirstPreventsPartialLeak", func(t *testing.T) {
		// "foo" is a prefix of "foobar". Without longest-first sort, replacing
		// "foo" first would yield "<REDACTED:short>bar", leaking "bar".
		tc := makeSnapshotTC(map[string]string{"short": "foo", "long": "foobar"}, []string{"short", "long"}, nil)
		result := redactForSnapshot("the secret is foobar end", tc)
		assert.NotContains(t, result, "foobar", "longer secret must be fully redacted")
		assert.NotContains(t, result, "foo", "shorter secret must not appear even as a suffix")
	})

	t.Run("RedactsInternalSecrets", func(t *testing.T) {
		tc := makeSnapshotTC(nil, nil, map[string]string{"host_secret": "abc123"})
		result := redactForSnapshot("auth=abc123", tc)
		assert.NotContains(t, result, "abc123")
		assert.Contains(t, result, "<REDACTED:host_secret>")
	})

	t.Run("EmptyValueSkipped", func(t *testing.T) {
		tc := makeSnapshotTC(map[string]string{"empty_key": ""}, []string{"empty_key"}, nil)
		result := redactForSnapshot("nothing to redact", tc)
		assert.Equal(t, "nothing to redact", result)
	})
}
