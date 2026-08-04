package util

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		// docker exec args include --env-file flag
		var envFileArg string
		for _, arg := range opts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				envFileArg = arg[11:]
			}
		}
		require.NotEmpty(t, envFileArg, "expected --env-file argument in docker exec args")

		// env file should exist and be readable
		content, err := os.ReadFile(envFileArg)
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
		hasEnvFile := false
		for _, arg := range opts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				hasEnvFile = true
			}
		}
		assert.True(t, hasEnvFile, "expected --env-file argument")
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("NicePrefixInContainerArgv", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", ""))
		// nice -n 0 must appear between the containerID and the original command.
		niceIdx := -1
		for i, arg := range opts.Args {
			if arg == "nice" {
				niceIdx = i
				break
			}
		}
		require.NotEqual(t, -1, niceIdx, "expected 'nice' in docker exec args")
		require.Less(t, niceIdx+2, len(opts.Args), "expected '-n' and '0' after 'nice'")
		assert.Equal(t, "-n", opts.Args[niceIdx+1])
		assert.Equal(t, fmt.Sprintf("%d", DefaultNice), opts.Args[niceIdx+2])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("SudoPrefixStrippedAndUserFlagAdded", func(t *testing.T) {
		// Jasper's SudoAs prepends ["sudo", "-u", user] to opts.Args before
		// WrapWithContainer is called. Verify the prefix is stripped and
		// --user=<user> is added to the docker exec flags instead.
		opts := &options.Create{Args: append([]string{"sudo", "-u", "ubuntu"}, baseArgs...)}
		require.NoError(t, WrapWithContainer(t.Context(), opts, "cid", "", ""))

		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])

		hasUser := false
		for _, arg := range opts.Args {
			assert.NotEqual(t, "sudo", arg, "sudo should not appear in final args")
			if arg == "--user=ubuntu" {
				hasUser = true
			}
		}
		assert.True(t, hasUser, "expected --user=ubuntu in docker exec args")
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
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

		envFilePath := ""
		for _, arg := range opts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				envFilePath = arg[11:]
			}
		}
		require.NotEmpty(t, envFilePath)
		fi, err := os.Stat(envFilePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	})

	t.Run("SeparateCommandsRetainTheirEnvironment", func(t *testing.T) {
		dir := t.TempDir()
		firstOpts := makeOpts()
		firstOpts.Environment = map[string]string{"COMMAND": "first"}
		require.NoError(t, WrapWithContainer(t.Context(), firstOpts, "cid", "", dir))

		firstEnvFile := ""
		for _, arg := range firstOpts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				firstEnvFile = arg[11:]
			}
		}
		require.NotEmpty(t, firstEnvFile)

		secondOpts := makeOpts()
		secondOpts.Environment = map[string]string{"COMMAND": "second"}
		require.NoError(t, WrapWithContainer(t.Context(), secondOpts, "cid", "", dir))
		secondEnvFile := ""
		for _, arg := range secondOpts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				secondEnvFile = arg[11:]
			}
		}
		require.NotEmpty(t, secondEnvFile)
		assert.NotEqual(t, firstEnvFile, secondEnvFile)

		data, err := os.ReadFile(firstEnvFile)
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
}
