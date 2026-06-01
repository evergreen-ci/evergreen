package util

import (
	"os"
	"path/filepath"
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
		require.NoError(t, WrapWithContainer(opts, "", "", ""))
		assert.Equal(t, baseArgs, opts.Args)
	})

	t.Run("ContainerIDOnly", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(opts, "abc123def", "", ""))
		require.GreaterOrEqual(t, len(opts.Args), len(baseArgs)+3)
		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "abc123def", opts.Args[2])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("WithWorkdir", func(t *testing.T) {
		opts := makeOpts()
		require.NoError(t, WrapWithContainer(opts, "abc123", "/data/mci/task1", ""))
		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "--workdir=/data/mci/task1", opts.Args[2])
		assert.Equal(t, "abc123", opts.Args[3])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("WithEnvFile", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"FOO": "bar", "SECRET": "s3cr3t"}
		require.NoError(t, WrapWithContainer(opts, "abc123", "", dir))

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
		require.NoError(t, WrapWithContainer(opts, "cid", "/work", dir))

		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "--workdir=/work", opts.Args[2])
		hasEnvFile := false
		for _, arg := range opts.Args {
			if len(arg) > 11 && arg[:11] == "--env-file=" {
				hasEnvFile = true
			}
		}
		assert.True(t, hasEnvFile, "expected --env-file argument")
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("PreservesOriginalArgs", func(t *testing.T) {
		opts := makeOpts()
		original := append([]string{}, opts.Args...)
		require.NoError(t, WrapWithContainer(opts, "xyz789", "", ""))
		assert.Equal(t, original, opts.Args[len(opts.Args)-len(original):])
	})

	t.Run("EnvFileMode0600", func(t *testing.T) {
		dir := t.TempDir()
		opts := makeOpts()
		opts.Environment = map[string]string{"KEY": "value"}
		require.NoError(t, WrapWithContainer(opts, "cid", "", dir))

		envFilePath := filepath.Join(dir, containerEnvFileName)
		fi, err := os.Stat(envFilePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	})
}

func TestWriteEnvFile(t *testing.T) {
	t.Run("WritesKeyValuePairs", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".evg-env")
		env := map[string]string{"A": "1", "B": "hello world"}
		require.NoError(t, writeEnvFile(path, env))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, "A=1\n")
		assert.Contains(t, body, "B=hello world\n")
	})

	t.Run("SkipsMultilineValues", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".evg-env")
		env := map[string]string{"GOOD": "ok", "BAD": "line1\nline2"}
		require.NoError(t, writeEnvFile(path, env))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		body := string(data)
		assert.Contains(t, body, "GOOD=ok\n")
		assert.NotContains(t, body, "BAD=")
	})

	t.Run("EmptyEnv", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".evg-env")
		require.NoError(t, writeEnvFile(path, nil))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Empty(t, string(data))
	})
}
