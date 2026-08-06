package agent

import (
	"context"
	"maps"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	agentcontainer "github.com/evergreen-ci/evergreen/agent/container"
	"github.com/evergreen-ci/evergreen/agent/internal"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func makeSnapshotTC(expansions map[string]string, redacted []string, secrets map[string]string) *taskContext {
	exp := util.Expansions{}
	maps.Copy(exp, expansions)
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
	t.Run("NilTCFailsClosed", func(t *testing.T) {
		assert.Equal(t, redactionUnavailable, redactForSnapshot("hello world", nil),
			"without a task config the redactor cannot know the secrets, so it must not emit the raw value")
	})

	t.Run("NilTaskConfigFailsClosed", func(t *testing.T) {
		assert.Equal(t, redactionUnavailable, redactForSnapshot("secret", &taskContext{}))
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
	destroyErr     error
	destroySignal  chan error
}

func (f *fakeContainer) GetID() string             { return f.id }
func (f *fakeContainer) GetName() string           { return f.name }
func (f *fakeContainer) GetEnvFileHostDir() string { return f.envFileHostDir }
func (f *fakeContainer) Destroy(ctx context.Context) error {
	f.destroyCalled++
	if f.destroySignal != nil {
		f.destroySignal <- ctx.Err()
	}
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

// TestMaybeStartContainerFailsClosedWhenDirsCannotBeSecured verifies that a
// distro requiring isolation does not start a container when neither ownership
// transfer nor the permissive fallback could be applied.
func TestMaybeStartContainerFailsClosedWhenDirsCannotBeSecured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Container isolation uses POSIX ownership and is not supported on Windows")
	}
	usr, err := user.Current()
	require.NoError(t, err)

	ctx := t.Context()
	a := agentForContainerTest()
	factoryCalled := false
	a.containerFactory = func(_ context.Context, _ agentcontainer.Config) (ContainerHandle, error) {
		factoryCalled = true
		return &fakeContainer{id: "id", name: "ctr"}, nil
	}
	conf := &internal.TaskConfig{
		// A workdir that does not exist defeats ownership transfer and the
		// fallback alike.
		WorkDir: filepath.Join(t.TempDir(), "absent"),
		Distro: &apimodels.DistroView{
			ExecUser: usr.Username,
			ContainerIsolation: &apimodels.ContainerIsolationSettings{
				Image:            "ubuntu:22.04",
				RequireIsolation: true,
			},
		},
		Task: task.Task{Id: "task-1"},
	}

	err = a.maybeStartContainer(ctx, conf, nil)
	require.Error(t, err, "require_isolation must fail closed when task directories cannot be secured at all")
	assert.False(t, factoryCalled, "container must not be created after directory preparation fails")
	assert.Nil(t, a.currentContainer)
}

// TestMaybeStartContainerUsesPermissiveFallbackWhenOwnershipFails verifies the
// task still runs when the exec user cannot be reconciled, which is the
// behaviour the pre-review implementation had unconditionally.
func TestMaybeStartContainerUsesPermissiveFallbackWhenOwnershipFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Container isolation uses POSIX ownership and is not supported on Windows")
	}
	ctx := t.Context()
	a := agentForContainerTest()
	created := &fakeContainer{id: "id", name: "ctr"}
	a.containerFactory = func(_ context.Context, _ agentcontainer.Config) (ContainerHandle, error) {
		return created, nil
	}
	workDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "tmp"), 0700))
	conf := &internal.TaskConfig{
		WorkDir: workDir,
		Distro: &apimodels.DistroView{
			ExecUser: "evergreen-nonexistent-user",
			ContainerIsolation: &apimodels.ContainerIsolationSettings{
				Image:            "ubuntu:22.04",
				RequireIsolation: true,
			},
		},
		Task: task.Task{Id: "task-1"},
	}

	require.NoError(t, a.maybeStartContainer(ctx, conf, nil),
		"an unreconcilable exec user must degrade rather than fail the task")
	assert.Equal(t, created, a.currentContainer)

	info, err := os.Stat(workDir)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0002, "the fallback must leave the workdir writable by the container")
}

func TestDestroyContainerRetentionWaitsForDeadline(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{
		id:            "abc",
		name:          "ctr",
		destroySignal: make(chan error, 1),
	}
	a := makeAgentWithFakeContainer(fc)
	a.retainContainerUntil = time.Now().Add(200 * time.Millisecond)
	conf := &internal.TaskConfig{ContainerID: "abc"}

	a.destroyContainer(ctx, conf)

	assert.Nil(t, a.currentContainer, "currentContainer must be cleared even in retention path")
	assert.Empty(t, conf.ContainerID, "conf.ContainerID must be cleared in retention path")

	// The point of retention is that the container survives for inspection.
	// Without this assertion the test would pass even if retention were ignored.
	select {
	case <-fc.destroySignal:
		t.Fatal("container was destroyed before its retention deadline elapsed")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case destroyCtxErr := <-fc.destroySignal:
		assert.NoError(t, destroyCtxErr, "retained container cleanup must not inherit cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("retained container was not destroyed after its retention deadline")
	}
}

func TestDestroyContainerRetentionEndsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	fc := &fakeContainer{
		id:            "abc",
		name:          "ctr",
		destroySignal: make(chan error, 1),
	}
	a := makeAgentWithFakeContainer(fc)
	a.retainContainerUntil = time.Now().Add(time.Hour)
	conf := &internal.TaskConfig{ContainerID: "abc"}

	a.destroyContainer(ctx, conf)

	select {
	case <-fc.destroySignal:
		t.Fatal("container was destroyed before shutdown was signalled")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case destroyCtxErr := <-fc.destroySignal:
		assert.NoError(t, destroyCtxErr, "shutdown cleanup must use a detached context")
	case <-time.After(5 * time.Second):
		t.Fatal("retained container was not destroyed after context cancellation")
	}
}

func TestDestroyContainerCallsDestroyEvenWithCancelledContext(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{id: "abc", name: "ctr", envFileHostDir: "/tmp/env"}
	a := makeAgentWithFakeContainer(fc)
	conf := &internal.TaskConfig{ContainerID: "abc", EnvFileHostDir: "/tmp/env"}

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	a.destroyContainer(cancelledCtx, conf)

	assert.Equal(t, 1, fc.destroyCalled, "the call site must not short-circuit on ctx.Err()")
	assert.Nil(t, a.currentContainer)
	assert.Empty(t, conf.ContainerID)
}

func TestDestroyContainerStillClearsReferenceOnDestroyError(t *testing.T) {
	ctx := t.Context()
	fc := &fakeContainer{
		id:         "abc",
		name:       "ctr",
		destroyErr: errors.New("docker daemon unreachable"),
	}
	a := makeAgentWithFakeContainer(fc)
	conf := &internal.TaskConfig{ContainerID: "abc"}

	a.destroyContainer(ctx, conf)

	assert.Equal(t, 1, fc.destroyCalled, "Destroy must be called once")
	assert.Nil(t, a.currentContainer, "currentContainer must be cleared even on Destroy error")
	assert.Empty(t, conf.ContainerID)
}

func TestIsAgentOwnedContainer(t *testing.T) {
	ownedLabels := map[string]string{agentcontainer.OwnerLabel: agentcontainer.OwnerLabelValue}

	t.Run("OwnerLabelMatches", func(t *testing.T) {
		assert.True(t, isAgentOwnedContainer(ownedLabels, nil))
	})

	t.Run("ForeignOwnerLabelIsProtectedEvenWithMatchingName", func(t *testing.T) {
		// An explicit foreign owner must win over the name prefix, otherwise
		// the reaper would destroy another system's container.
		assert.False(t, isAgentOwnedContainer(
			map[string]string{agentcontainer.OwnerLabel: "someone-else"},
			[]string{"/" + agentcontainer.ContainerNamePrefix + "abc"},
		))
	})

	t.Run("UnlabelledContainerWithNamePrefixIsReaped", func(t *testing.T) {
		// Containers created before ownership labels existed must still be
		// reaped, or an agent rollout strands orphans on the host forever.
		assert.True(t, isAgentOwnedContainer(nil, []string{"/" + agentcontainer.ContainerNamePrefix + "abc"}))
	})

	t.Run("NamePrefixIsAnchoredNotSubstring", func(t *testing.T) {
		// Docker's own name filter is an unanchored substring match, which
		// would have destroyed a container a human named this way.
		assert.False(t, isAgentOwnedContainer(nil, []string{"/my-" + agentcontainer.ContainerNamePrefix + "debug"}))
	})

	t.Run("UnrelatedUnlabelledContainerIsNotReaped", func(t *testing.T) {
		assert.False(t, isAgentOwnedContainer(nil, []string{"/postgres"}))
		assert.False(t, isAgentOwnedContainer(nil, nil))
	})
}

func TestContainerNames(t *testing.T) {
	assert.Equal(t, "a,b", containerNames([]string{"/a", "/b"}))
	assert.Empty(t, containerNames(nil))
}

func TestContainerToolchainDirsDoesNotMountAllOfOpt(t *testing.T) {
	assert.NotContains(t, containerToolchainDirs, "/opt",
		"mounting all of /opt exposes anything provisioning writes there to every containerized task")
}

func TestToolchainMountsOnlyIncludesExistingDirsReadOnly(t *testing.T) {
	existing := t.TempDir()
	missing := filepath.Join(t.TempDir(), "absent")

	original := containerToolchainDirs
	t.Cleanup(func() { containerToolchainDirs = original })
	containerToolchainDirs = []string{existing, missing}

	mounts := toolchainMounts(t.Context(), "task-1")

	require.Len(t, mounts, 1, "a nonexistent source would make Docker reject container creation")
	assert.Equal(t, existing, mounts[0].Source)
	assert.Equal(t, existing, mounts[0].Target)
	assert.True(t, mounts[0].ReadOnly, "toolchain mounts must be read-only")
}

func TestSecureContainerDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Container isolation uses POSIX ownership and is not supported on Windows")
	}

	t.Run("EmptyWorkDirIsNoop", func(t *testing.T) {
		fellBack, err := secureContainerDirs("", "someuser")
		assert.NoError(t, err)
		assert.False(t, fellBack)
	})

	t.Run("EmptyExecUserIsNoop", func(t *testing.T) {
		// Without an exec user, docker exec runs as root in the container and
		// can already write to the agent-owned directories.
		fellBack, err := secureContainerDirs(t.TempDir(), "")
		assert.NoError(t, err)
		assert.False(t, fellBack)
	})

	t.Run("UnknownExecUserFallsBackToWritableDirs", func(t *testing.T) {
		// The container must still be able to write its own workdir, so an
		// unreconcilable exec user degrades rather than breaking the task.
		workDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(workDir, "tmp"), 0700))

		fellBack, err := secureContainerDirs(workDir, "evergreen-nonexistent-user")

		require.NoError(t, err)
		assert.True(t, fellBack, "the caller must be told the directories are world-writable")
		for _, dir := range []string{workDir, filepath.Join(workDir, "tmp")} {
			info, statErr := os.Stat(dir)
			require.NoError(t, statErr)
			assert.NotZero(t, info.Mode().Perm()&0002, "the fallback must leave %s writable by the container", dir)
		}
	})

	t.Run("NonexistentWorkDirErrorsRatherThanSilentlyDegrading", func(t *testing.T) {
		usr, err := user.Current()
		require.NoError(t, err)

		fellBack, err := secureContainerDirs(filepath.Join(t.TempDir(), "absent"), usr.Username)

		require.Error(t, err, "when neither ownership nor the fallback can be applied the caller must hear about it")
		assert.False(t, fellBack)
	})

	t.Run("CurrentUserSetsNonWorldWritableMode", func(t *testing.T) {
		usr, err := user.Current()
		require.NoError(t, err)

		workDir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(workDir, "tmp"), 0700))

		fellBack, err := secureContainerDirs(workDir, usr.Username)
		require.NoError(t, err)
		assert.False(t, fellBack, "ownership transfer succeeded, so no fallback should be reported")

		for _, dir := range []string{workDir, filepath.Join(workDir, "tmp")} {
			info, statErr := os.Stat(dir)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(containerDirMode), info.Mode().Perm(), "mode should be %o for %s", containerDirMode, dir)
			assert.Zero(t, info.Mode().Perm()&0002, "task directories must not be world-writable when ownership succeeds")
		}
	})
}

func TestChownContainerDirRefusesSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "link")
	require.NoError(t, os.Mkdir(target, 0700))
	require.NoError(t, os.Symlink(target, link))

	err := chownContainerDir(link, os.Getuid(), os.Getgid())

	require.Error(t, err, "following a symlink would let a local user redirect the chown")
	assert.Contains(t, err.Error(), "symlink")

	info, err := os.Stat(target)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		assert.True(t, info.IsDir(), "the symlink target must remain a directory")
		return
	}
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "the symlink target must be untouched")
}

func TestChownContainerDirRefusesNonDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0600))

	assert.Error(t, chownContainerDir(file, os.Getuid(), os.Getgid()))
}

func TestReadLatestContainerEnvFileReadsNewestUniqueFile(t *testing.T) {
	dir := t.TempDir()
	olderPath := filepath.Join(dir, containerEnvFilePrefix+"older")
	newerPath := filepath.Join(dir, containerEnvFilePrefix+"newer")
	require.NoError(t, os.WriteFile(olderPath, []byte("VALUE=older\n"), 0600))
	require.NoError(t, os.WriteFile(newerPath, []byte("VALUE=newer\n"), 0600))
	baseTime := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(olderPath, baseTime, baseTime))
	require.NoError(t, os.Chtimes(newerPath, baseTime.Add(time.Second), baseTime.Add(time.Second)))

	data, err := readLatestContainerEnvFile(dir)
	require.NoError(t, err)
	assert.Equal(t, "VALUE=newer\n", string(data))
}

func TestContainerEnvFileKeysOmitsValues(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, containerEnvFilePrefix+"1"),
		[]byte("AWS_SECRET_ACCESS_KEY=supersecret\nPATH=/usr/bin\n"),
		0600,
	))

	keys, err := containerEnvFileKeys(dir)
	require.NoError(t, err)

	assert.Equal(t, []string{"AWS_SECRET_ACCESS_KEY", "PATH"}, keys)
	for _, key := range keys {
		assert.NotContains(t, key, "supersecret", "env values must never reach telemetry")
	}
}

func TestParseHostEnv(t *testing.T) {
	sentinel := hostEnvSentinel + "\x00"

	t.Run("DiscardsProfileOutputBeforeSentinel", func(t *testing.T) {
		out := []byte("Welcome to the machine\nMOTD banner\n" + sentinel + "PATH=/usr/bin\x00")
		assert.Equal(t, []string{"PATH=/usr/bin"}, parseHostEnv(out))
	})

	t.Run("PreservesSpacesAndGlobCharacters", func(t *testing.T) {
		// The previous unquoted `echo '%s='$VAR` word-split and glob-expanded
		// these values before they reached the env file.
		out := []byte(sentinel + "PATH=/opt/my tools/bin:/usr/*\x00")
		assert.Equal(t, []string{"PATH=/opt/my tools/bin:/usr/*"}, parseHostEnv(out))
	})

	t.Run("DropsValuesContainingNewlines", func(t *testing.T) {
		out := []byte(sentinel + "GOOD=1\x00BAD=line1\nline2\x00")
		assert.Equal(t, []string{"GOOD=1"}, parseHostEnv(out),
			"an env file cannot represent a newline, and Docker rejects the whole file")
	})

	t.Run("NoSentinelYieldsNothing", func(t *testing.T) {
		assert.Empty(t, parseHostEnv([]byte("PATH=/usr/bin\x00")))
	})

	t.Run("SkipsRecordsWithoutSeparatorOrKey", func(t *testing.T) {
		out := []byte(sentinel + "novalue\x00=orphan\x00OK=1\x00")
		assert.Equal(t, []string{"OK=1"}, parseHostEnv(out))
	})
}

func TestHostEnvShellCommandQuotesExpansions(t *testing.T) {
	cmd := hostEnvShellCommand()

	assert.Contains(t, cmd, hostEnvSentinel, "the sentinel must be emitted so profile output can be discarded")
	assert.Contains(t, cmd, `"$PATH"`, "expansions must be quoted to prevent word splitting and globbing")
	assert.NotContains(t, cmd, "echo '", "echo with an unquoted expansion is what corrupted values")
}

func TestFormatHostEnv(t *testing.T) {
	assert.Empty(t, formatHostEnv(nil))
	assert.Equal(t, "A=1\nB=2\n", formatHostEnv([]string{"A=1", "B=2"}))
}

func TestAgentHostEnvDropsNewlineValues(t *testing.T) {
	t.Setenv("GOROOT", "/opt/golang/go\nEVIL=1")
	t.Setenv("JAVA_HOME", "/opt/java")

	entries := agentHostEnv()

	assert.Contains(t, entries, "JAVA_HOME=/opt/java")
	for _, entry := range entries {
		assert.NotContains(t, entry, "EVIL", "a newline in a value must not smuggle an extra env entry")
	}
}

func TestWriteHostEnvFileReportsCaptureFailureAndWritesFallback(t *testing.T) {
	dir := t.TempDir()
	bashPath := filepath.Join(dir, "bash")
	require.NoError(t, os.WriteFile(bashPath, []byte("#!/bin/sh\n/bin/sleep 2\n/bin/touch \"$0.finished\"\n"), 0700))
	t.Setenv("PATH", dir)
	t.Setenv("JAVA_HOME", "/opt/java")

	envPath := filepath.Join(dir, "host-env")
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	t.Cleanup(cancel)

	err := writeHostEnvFile(ctx, envPath)

	require.Error(t, err, "a swallowed capture failure hides a degraded container environment")
	assert.NoFileExists(t, bashPath+".finished", "the timed-out login shell must not resume profile processing")

	data, readErr := os.ReadFile(envPath)
	require.NoError(t, readErr, "the fallback env file must still be written")
	assert.Contains(t, string(data), "JAVA_HOME=/opt/java", "the fallback should carry the agent's own environment")
}

func TestWriteHostEnvFileParsesLoginShellOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot execute the extensionless shell script used by this integration test")
	}

	dir := t.TempDir()
	bashPath := filepath.Join(dir, "bash")
	// Stand in for a login shell whose profile scripts print a banner before
	// the env dump, and whose PATH contains a space.
	script := "#!/bin/sh\n" +
		"printf 'MOTD banner\\n'\n" +
		"printf '%s\\0' '" + hostEnvSentinel + "'\n" +
		"printf '%s=%s\\0' 'PATH' '/opt/my tools/bin'\n"
	require.NoError(t, os.WriteFile(bashPath, []byte(script), 0700))
	t.Setenv("PATH", dir)

	envPath := filepath.Join(dir, "host-env")
	require.NoError(t, writeHostEnvFile(t.Context(), envPath))

	data, err := os.ReadFile(envPath)
	require.NoError(t, err)
	assert.Equal(t, "PATH=/opt/my tools/bin\n", string(data),
		"the banner must be discarded and the spaced value preserved verbatim")

	info, err := os.Stat(envPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}
