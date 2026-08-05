package util

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// envFilePaths returns the values of every --env-file flag in args, in order.
func envFilePaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		if path, ok := strings.CutPrefix(arg, "--env-file="); ok {
			paths = append(paths, path)
		}
	}
	return paths
}

func TestWrapWithContainer(t *testing.T) {
	baseArgs := []string{"/bin/bash", "-c", "echo hello"}

	makeOpts := func() *options.Create {
		return &options.Create{Args: append([]string{}, baseArgs...)}
	}

	t.Run("EmptyContainerID", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "", "", ""))
		assert.Equal(t, baseArgs, opts.Args)
	})

	t.Run("ContainerIDOnly", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "abc123def", "", ""))
		require.GreaterOrEqual(t, len(opts.Args), len(baseArgs)+4)
		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "-i", opts.Args[2])
		assert.Equal(t, "abc123def", opts.Args[3])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("WithWorkdir", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "abc123", "/data/mci/task1", ""))
		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "-i", opts.Args[2])
		assert.Equal(t, "--workdir=/data/mci/task1", opts.Args[3])
		assert.Equal(t, "abc123", opts.Args[4])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("WithEnvFile", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"FOO": "bar", "SECRET": "s3cr3t"}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "abc123", "", dir))

		paths := envFilePaths(opts.Args)
		require.Len(t, paths, 1)

		content, err := os.ReadFile(paths[0])
		require.NoError(t, err)
		body := string(content)
		assert.Contains(t, body, "FOO=bar\n")
		assert.Contains(t, body, "SECRET=s3cr3t\n")
	})

	t.Run("WithWorkdirAndEnvFile", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"K": "v"}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "/work", dir))

		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "-i", opts.Args[2])
		assert.Equal(t, "--workdir=/work", opts.Args[3])
		assert.Len(t, envFilePaths(opts.Args), 1)
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("NicePrefixAppliedOnlyWhenItChangesPriority", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", ""))

		// A `nice -n 0` prefix would be a no-op that forces every task image to
		// provide nice, so it is only emitted when DefaultNice is non-zero.
		expected := baseArgs
		if DefaultNice != 0 {
			expected = append([]string{"nice", "-n", strconv.Itoa(DefaultNice)}, baseArgs...)
		}
		require.Len(t, opts.Args, 4+len(expected))
		assert.Equal(t, expected, opts.Args[4:])
	})

	t.Run("SudoPrefixStrippedAndUserFlagAdded", func(t *testing.T) {
		// Jasper's SudoAs prepends ["sudo", "-u", user] to opts.Args before
		// WrapWithContainer is called. Verify the prefix is stripped and
		// --user=<user> is added to the docker exec flags instead.
		opts := &options.Create{Args: append([]string{"sudo", "-u", "ubuntu"}, baseArgs...)}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", ""))

		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])

		assert.NotContains(t, opts.Args, "sudo")
		userIdx := slices.Index(opts.Args, "--user=ubuntu")
		require.NotEqual(t, -1, userIdx, "expected --user=ubuntu in docker exec args")
		// The flag has to precede the container ID or docker parses it as an
		// argument to the in-container command instead.
		assert.Less(t, userIdx, slices.Index(opts.Args, "cid"))
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("HostEnvFilePrecedesPerCommandEnvFile", func(t *testing.T) {
		dir := t.TempDir()
		hostEnvPath := filepath.Join(dir, containerHostEnvFileName)
		require.NoError(t, os.WriteFile(hostEnvPath, []byte("PATH=/usr/bin\n"), 0600))

		opts := makeOpts()
		opts.Environment = map[string]string{"K": "v"}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", dir))

		// Docker applies --env-file args in order, so the per-command file must
		// come second for its values to win.
		paths := envFilePaths(opts.Args)
		require.Len(t, paths, 2)
		assert.Equal(t, hostEnvPath, paths[0])
		assert.NotEqual(t, hostEnvPath, paths[1])
	})

	t.Run("MissingHostEnvFileIsOmitted", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"K": "v"}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", dir))

		paths := envFilePaths(opts.Args)
		require.Len(t, paths, 1)
		assert.NotEqual(t, filepath.Join(dir, containerHostEnvFileName), paths[0])
	})

	t.Run("EmptyEnvironmentWritesNoEnvFile", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", dir))

		assert.Empty(t, envFilePaths(opts.Args))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, entries, "no env file should be created for an empty environment")
	})

	t.Run("EnvFileFailureLeavesArgsUnmodified", func(t *testing.T) {
		opts := makeOpts()
		opts.Environment = map[string]string{"K": "v"}
		require.Error(t, WrapWithContainer(t.Context(), opts, "cid", "", filepath.Join(t.TempDir(), "missing")))
		assert.Equal(t, baseArgs, opts.Args)
	})

	t.Run("PreservesOriginalArgs", func(t *testing.T) {
		opts := makeOpts()
		original := append([]string{}, opts.Args...)
		require.NoError(t, WrapWithContainer(t.Context(), opts, "xyz789", "", ""))
		assert.Equal(t, original, opts.Args[len(opts.Args)-len(original):])
	})

	t.Run("EnvFileMode0600", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			// Windows does not support POSIX permission bits and reports writable files with the default 0666 mode.
			t.Skip("Windows does not support POSIX permission bits")
		}

		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"KEY": "value"}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", dir))

		paths := envFilePaths(opts.Args)
		require.Len(t, paths, 1)
		fi, err := os.Stat(paths[0])
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	})

	t.Run("SeparateCommandsRetainTheirEnvironment", func(t *testing.T) {
		dir := t.TempDir()
		firstOpts := makeOpts()
		firstOpts.Environment = map[string]string{"COMMAND": "first"}
		require.NoError(t, WrapWithContainer(t.Context(), firstOpts, "cid", "", dir))

		firstPaths := envFilePaths(firstOpts.Args)
		require.Len(t, firstPaths, 1)

		secondOpts := makeOpts()
		secondOpts.Environment = map[string]string{"COMMAND": "second"}
		require.NoError(t, WrapWithContainer(t.Context(), secondOpts, "cid", "", dir))
		secondPaths := envFilePaths(secondOpts.Args)
		require.Len(t, secondPaths, 1)
		assert.NotEqual(t, firstPaths[0], secondPaths[0])

		data, err := os.ReadFile(firstPaths[0])
		require.NoError(t, err)
		assert.Contains(t, string(data), "COMMAND=first\n")
		assert.NotContains(t, string(data), "COMMAND=second\n")
	})
}

func TestWriteEnvFile(t *testing.T) {
	t.Run("WritesKeyValuePairs", func(t *testing.T) {
		dir := t.TempDir()
		env := map[string]string{"A": "1", "B": "hello world"}
		path, err := writeEnvFile(t.Context(), dir, env)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, "A=1\n")
		assert.Contains(t, body, "B=hello world\n")
	})

	t.Run("SkipsMultilineValues", func(t *testing.T) {
		dir := t.TempDir()
		env := map[string]string{"GOOD": "ok", "BAD": "line1\nline2"}
		path, err := writeEnvFile(t.Context(), dir, env)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, "GOOD=ok\n")
		assert.NotContains(t, body, "BAD=")
	})

	t.Run("EmptyEnv", func(t *testing.T) {
		dir := t.TempDir()
		path, err := writeEnvFile(t.Context(), dir, nil)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Empty(t, string(data))
	})

	t.Run("NonexistentDirErrors", func(t *testing.T) {
		_, err := writeEnvFile(t.Context(), filepath.Join(t.TempDir(), "missing"), map[string]string{"A": "1"})
		assert.Error(t, err)
	})
}
