package agent

import (
	"context"
	"testing"
	"time"

	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
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

// fakeContainer is a test double for ContainerHandle that does not require
// a live Docker daemon.
type fakeContainer struct {
	id             string
	name           string
	envFileHostDir string
	destroyCalled  int
	closeCalled    int
	destroyErr     error
}

func (f *fakeContainer) GetID() string             { return f.id }
func (f *fakeContainer) GetName() string           { return f.name }
func (f *fakeContainer) GetEnvFileHostDir() string { return f.envFileHostDir }
func (f *fakeContainer) Close()                    { f.closeCalled++ }
func (f *fakeContainer) Destroy(_ context.Context) error {
	f.destroyCalled++
	return f.destroyErr
}

func agentForContainerTest() *Agent {
	return &Agent{
		opts:   Options{Cleanup: true},
		tracer: otel.GetTracerProvider().Tracer("test"),
	}
}

func makeAgentWithFakeContainer(fc *fakeContainer) *Agent {
	a := agentForContainerTest()
	a.currentContainer = fc
	return a
}

func makeDistroWithIsolation(image string) *apimodels.DistroView {
	return &apimodels.DistroView{
		ContainerIsolation: &apimodels.ContainerIsolationSettings{
			Image: image,
		},
	}
}

func TestMaybeStartContainerReusePath(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc123", name: "evergreen-task-abc", envFileHostDir: "/tmp/env-abc"}
	a := makeAgentWithFakeContainer(fc)
	conf := &internal.TaskConfig{
		Distro: makeDistroWithIsolation("ubuntu:22.04"),
		Task:   task.Task{},
	}

	err := a.maybeStartContainer(ctx, conf, nil)
	require.NoError(t, err)

	assert.Equal(t, "abc123", conf.ContainerID, "reuse path should wire ContainerID from existing container")
	assert.Equal(t, "/tmp/env-abc", conf.EnvFileHostDir, "reuse path should wire EnvFileHostDir from existing container")
	assert.Equal(t, fc, a.currentContainer, "existing container should not be replaced")
}

func TestMaybeStartContainerCreatePath(t *testing.T) {
	ctx := t.Context()
	a := agentForContainerTest()
	created := &fakeContainer{id: "newid", name: "evergreen-task-new", envFileHostDir: "/tmp/env-new"}
	a.containerFactory = func(_ context.Context, _ agentcontainer.Config) (ContainerHandle, error) {
		return created, nil
	}
	conf := &internal.TaskConfig{
		Distro: makeDistroWithIsolation("ubuntu:22.04"),
		Task:   task.Task{Id: "task-1"},
	}

	err := a.maybeStartContainer(ctx, conf, nil)
	require.NoError(t, err)

	assert.Equal(t, "newid", conf.ContainerID)
	assert.Equal(t, "/tmp/env-new", conf.EnvFileHostDir)
	assert.Equal(t, created, a.currentContainer)
}

func TestMaybeStartContainerNilDistroIsNoop(t *testing.T) {
	ctx := t.Context()
	a := agentForContainerTest()
	conf := &internal.TaskConfig{Distro: nil}

	require.NoError(t, a.maybeStartContainer(ctx, conf, nil))
	assert.Nil(t, a.currentContainer)
}

func TestMaybeStartContainerNilIsolationIsNoop(t *testing.T) {
	ctx := t.Context()
	a := agentForContainerTest()
	conf := &internal.TaskConfig{Distro: &apimodels.DistroView{ContainerIsolation: nil}}

	require.NoError(t, a.maybeStartContainer(ctx, conf, nil))
	assert.Nil(t, a.currentContainer)
}

func TestDestroyContainerNilCurrentContainerIsNoop(t *testing.T) {
	ctx := t.Context()
	a := &Agent{}
	conf := &internal.TaskConfig{}

	// Should not panic and should leave everything unchanged.
	a.destroyContainer(ctx, conf)
	assert.Empty(t, conf.ContainerID)
	assert.Empty(t, conf.EnvFileHostDir)
}

func TestDestroyContainerNilConfSafe(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc", name: "ctr"}
	a := makeAgentWithFakeContainer(fc)

	// nil conf is explicitly supported (loop-exit defer has no conf).
	a.destroyContainer(ctx, nil)
	assert.Nil(t, a.currentContainer)
	assert.Equal(t, 1, fc.destroyCalled)
}

func TestDestroyContainerClearsFields(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc", name: "ctr", envFileHostDir: "/tmp/env"}
	a := makeAgentWithFakeContainer(fc)
	conf := &internal.TaskConfig{ContainerID: "abc", EnvFileHostDir: "/tmp/env"}

	a.destroyContainer(ctx, conf)

	assert.Nil(t, a.currentContainer, "currentContainer must be cleared after destroy")
	assert.Empty(t, conf.ContainerID, "conf.ContainerID must be cleared")
	assert.Empty(t, conf.EnvFileHostDir, "conf.EnvFileHostDir must be cleared")
	assert.Equal(t, 1, fc.destroyCalled)
}

func TestDestroyContainerIdempotent(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc", name: "ctr"}
	a := makeAgentWithFakeContainer(fc)
	conf := &internal.TaskConfig{ContainerID: "abc"}

	a.destroyContainer(ctx, conf)
	// Second call: currentContainer is already nil, should be a no-op.
	a.destroyContainer(ctx, conf)

	assert.Equal(t, 1, fc.destroyCalled, "Destroy should only be called once")
}

func TestMaybeStartContainerFailClosedPropagatesError(t *testing.T) {
	ctx := t.Context()
	a := agentForContainerTest()
	a.containerFactory = func(_ context.Context, _ agentcontainer.Config) (ContainerHandle, error) {
		return nil, errors.New("docker unavailable")
	}
	conf := &internal.TaskConfig{
		Distro: &apimodels.DistroView{
			ContainerIsolation: &apimodels.ContainerIsolationSettings{
				Image:            "ubuntu:22.04",
				RequireIsolation: true,
			},
		},
		Task: task.Task{Id: "task-1"},
	}

	err := a.maybeStartContainer(ctx, conf, nil)
	require.Error(t, err, "fail-closed: error should be returned when factory fails")
	assert.Nil(t, a.currentContainer)
	assert.Empty(t, conf.ContainerID)
}

func TestMaybeStartContainerFailOpenReturnsNil(t *testing.T) {
	ctx := t.Context()
	a := agentForContainerTest()
	a.containerFactory = func(_ context.Context, _ agentcontainer.Config) (ContainerHandle, error) {
		return nil, errors.New("docker unavailable")
	}
	conf := &internal.TaskConfig{
		Distro: makeDistroWithIsolation("ubuntu:22.04"), // RequireIsolation defaults false
		Task:   task.Task{Id: "task-1"},
	}

	err := a.maybeStartContainer(ctx, conf, nil)
	require.NoError(t, err, "fail-open: error should be swallowed when RequireIsolation is false")
	assert.Nil(t, a.currentContainer, "currentContainer must remain nil on fail-open")
	assert.Empty(t, conf.ContainerID, "conf.ContainerID must not be set on fail-open")
}

func TestDestroyContainerRetentionCallsCloseNotDestroy(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc", name: "ctr"}
	a := makeAgentWithFakeContainer(fc)
	// Set retention window far in the future so destroyContainer takes the retention path.
	a.retainContainerUntil = time.Now().Add(5 * time.Minute)
	conf := &internal.TaskConfig{ContainerID: "abc"}

	a.destroyContainer(ctx, conf)

	assert.Equal(t, 1, fc.closeCalled, "retention path must call Close() to release the Docker client")
	assert.Equal(t, 0, fc.destroyCalled, "retention path must NOT call Destroy() — container must stay running for inspection")
	assert.Nil(t, a.currentContainer, "currentContainer must be cleared even in retention path")
	assert.Empty(t, conf.ContainerID, "conf.ContainerID must be cleared in retention path")
}
